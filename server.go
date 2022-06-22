package main

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"github.com/golang-jwt/jwt"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	vault "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/certutil"
	pb "github.com/jamiewhitney/grpc-go-vault/hello"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"net"
	"net/http"
	"os"
	"strings"
)

type server struct {
	pb.UnimplementedHelloServiceServer
}

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errInvalidScope    = status.Errorf(codes.Unauthenticated, "invalid scope")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
	errInvalidAudience = status.Errorf(codes.Unauthenticated, "invalid audience")
	errInvalidIssuer   = status.Errorf(codes.Unauthenticated, "invalid issuer")
	errInvalidSubject  = status.Errorf(codes.Unauthenticated, "invalid subject")
	errMissingToken    = status.Errorf(codes.NotFound, "missing token")
	authToken          string
	Key                *rsa.PublicKey
)

func main() {

	//vault

	vaultClient, err := vault.NewClient(&vault.Config{
		Address: os.Getenv("VAULT_ADDR"),
	})
	if err != nil {
		fmt.Printf("failed to create vault client: %v", err)
	}

	vaultClient.SetToken("root")

	secret, err := vaultClient.Logical().Write("grpc/issue/hello-service", map[string]interface{}{
		"common_name": "grpc.example.com",
		"alt_names":   "localhost",
	})
	if err != nil {
		fmt.Printf("failed to create certificate: %v", err)
	}

	// tls credentials

	parsedCertBundle, err := certutil.ParsePKIMap(secret.Data)
	if err != nil {
		log.Errorf("errror parsing secret: %s", err)
	}

	tlsConfig, err := parsedCertBundle.GetTLSConfig(certutil.TLSServer)
	if err != nil {
		log.Errorf("failed to get TLS config: %s", err)
	}

	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	tlsCredentials := credentials.NewTLS(tlsConfig)

	// JWT Public Cert
	keyData, err := vaultClient.Logical().Read("hello-service/data/auth0")
	if err != nil {
		log.Error(err)
	}

	keyParse := keyData.Data["data"].(map[string]interface{})

	keyYeah := keyParse["public_cert"].(string)

	Key, err = jwt.ParseRSAPublicKeyFromPEM([]byte(keyYeah))
	if err != nil {
		log.Error(err)
	}

	// metrics
	reg := prometheus.NewRegistry()
	grpcMetrics := grpc_prometheus.NewServerMetrics()
	customizedCounterMetric := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "demo_server_say_hello_method_handle_count",
		Help: "Total number of RPCs handled on the server.",
	}, []string{"name"})
	// Register client metrics to registry.
	reg.MustRegister(grpcMetrics, customizedCounterMetric)

	httpServer := &http.Server{Handler: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), Addr: fmt.Sprintf(":%d", 9094)}

	// Start your http server for prometheus.
	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatal("Unable to start a http server.")
		}
	}()
	// grpc server

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 3000))
	if err != nil {
		fmt.Printf("failed to listen: %v", err)
	}
	s := grpc.NewServer(grpc.Creds(tlsCredentials), grpc.ChainUnaryInterceptor(ensureValidToken, grpcMetrics.UnaryServerInterceptor()))
	pb.RegisterHelloServiceServer(s, &server{})

	if err := s.Serve(lis); err != nil {
		fmt.Printf("failed to serve: %s", err)
	}
}

func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloRequest, error) {
	fmt.Printf("Received: %v\n", in.GetName())
	return &pb.HelloRequest{Name: "Hello " + in.GetName()}, nil
}

type MyCustomClaims struct {
	Scope string `json:"scope"`
	jwt.StandardClaims
}

func ensureValidToken(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errMissingMetadata
	}

	token, claims, err := ParseToken(md["authorization"])
	if err != nil {
		log.Errorf("failed to parse token: %s", err)
	}

	if !token.Valid {
		return nil, errInvalidToken
	}

	if !claims.HasScope(os.Getenv("AUTH0_SCOPE")) {
		return nil, errInvalidScope
	}

	if !claims.VerifyAudience(os.Getenv("AUTH0_AUDIENCE"), true) {
		return nil, errInvalidAudience
	}

	if !claims.VerifyIssuer(os.Getenv("AUTH0_ISSUER"), true) {
		return nil, errInvalidIssuer
	}

	if claims.Subject != os.Getenv("AUTH0_SUBJECT") {
		return nil, errInvalidSubject
	}

	return handler(ctx, req)
}

func ParseToken(authorization []string) (*jwt.Token, *MyCustomClaims, error) {
	if len(authorization) < 1 {
		return nil, nil, errMissingToken
	}
	accessToken := strings.TrimPrefix(authorization[0], "Bearer ")

	claimsStruct := MyCustomClaims{}
	token, err := jwt.ParseWithClaims(
		accessToken,
		&claimsStruct,
		func(token *jwt.Token) (interface{}, error) {
			_, ok := token.Method.(*jwt.SigningMethodRSA)
			if !ok {
				return nil, fmt.Errorf("unexpected token signing method")
			}

			return Key, nil
		},
	)
	if err != nil {
		return nil, nil, errInvalidToken
	}

	return token, &claimsStruct, nil
}

func (c MyCustomClaims) HasScope(expectedScope string) bool {
	result := strings.Split(c.Scope, " ")
	for i := range result {
		if result[i] == expectedScope {
			return true
		}
	}

	return false
}
