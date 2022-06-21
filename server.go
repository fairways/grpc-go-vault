package main

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"github.com/gocql/gocql"
	"github.com/golang-jwt/jwt"
	vault "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/certutil"
	pb "github.com/jamiewhitney/grpc-go-vault/hello"
	"github.com/prometheus/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"net"
	"strings"
)

type server struct {
	pb.UnimplementedHelloServiceServer
}

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
	authToken          string
	Key                *rsa.PublicKey
	DB                 *gocql.Session
)

func main() {
	cluster := gocql.NewCluster("localhost")
	cluster.Keyspace = "example"
	cluster.Authenticator = gocql.PasswordAuthenticator{
		Username: "cassandra",
		Password: "cassandra",
	}
	var err error
	DB, err = cluster.CreateSession()
	if err != nil {
		log.Error(err)
	}
	//vault

	vaultClient, err := vault.NewClient(&vault.Config{
		Address: "http://localhost:8200",
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
		fmt.Errorf("Error parsing secret: %s", err)
	}

	tlsConfig, err := parsedCertBundle.GetTLSConfig(certutil.TLSServer)
	if err != nil {
		fmt.Errorf("Could not get TLS config: %s", err)
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
	fmt.Println(Key)

	// grpc server

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 3000))
	if err != nil {
		fmt.Printf("failed to listen: %v", err)
	}
	s := grpc.NewServer(grpc.Creds(tlsCredentials), grpc.UnaryInterceptor(ensureValidToken))
	pb.RegisterHelloServiceServer(s, &server{})

	if err := s.Serve(lis); err != nil {
		fmt.Printf("failed to serve: %s", err)
	}
}

func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloRequest, error) {
	fmt.Printf("Received: %v\n", in.GetName())

	if err := DB.Query(`INSERT into users(id, first_name) values(uuid(), ?)`, in.GetName()).Exec(); err != nil {
		return &pb.HelloRequest{Name: "not inserted"}, err
	}
	return &pb.HelloRequest{Name: "Inserted record"}, nil
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
	if !valid(md["authorization"]) {
		return nil, errInvalidToken
	}
	return handler(ctx, req)
}

func valid(authorization []string) bool {
	if len(authorization) < 1 {
		return false
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
		fmt.Errorf("invalid token: %w", err)
	}

	claims, _ := token.Claims.(*MyCustomClaims)
	fmt.Println(claims)
	if !claimsStruct.HasScope("read:messages") {
		fmt.Println("forbidden")
		return false
	}
	fmt.Println("access granted")
	return true
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
