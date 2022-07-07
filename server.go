package main

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"github.com/gocql/gocql"
	"github.com/golang-jwt/jwt"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	vault "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/certutil"
	"github.com/jamiewhitney/auth-jwt-grpc"
	pb "github.com/jamiewhitney/grpc-go-vault/hello"
	"github.com/newrelic/go-agent/v3/integrations/nrgrpc"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"net"

	"os"
	"time"
)

type server struct {
	pb.UnimplementedHelloServiceServer
}

var (
	Key                *rsa.PublicKey
	app                *newrelic.Application
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errInvalidScope    = status.Errorf(codes.Unauthenticated, "invalid scope")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
)

func main() {
	log := logrus.New()
	log.Formatter = &logrus.JSONFormatter{}

	//tracing
	app, err := newrelic.NewApplication(
		newrelic.ConfigAppName("gRPC Server"),
		newrelic.ConfigLicense(MustMapEnv("NEWRELIC_API_KEY")),
		func(config *newrelic.Config) {
			config.Labels = map[string]string{
				"environment": "production",
				"region":      "eu-west-1",
			}
		},
	)
	if err != nil {
		log.Error(err)
	}
	log.Infof("waiting for connection %s,", time.Now())
	if err := app.WaitForConnection(30 * time.Second); err != nil {
		log.Error(err)
	}
	log.Infof("connected %s,", time.Now())
	////vault

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
		log.Errorf("Error parsing secret: %s", err)
	}

	tlsConfig, err := parsedCertBundle.GetTLSConfig(certutil.TLSServer)
	if err != nil {
		log.Errorf("Could not get TLS config: %v", err)
	}

	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	tlsCredentials := credentials.NewTLS(tlsConfig)

	// JWT Public Cert
	keyData, err := vaultClient.Logical().Read("hello-service/data/auth0")
	if err != nil {
		log.Error(err)
	}

	keyParse := keyData.Data["data"].(map[string]interface{})

	keyYeah := keyParse["pem"].(string)

	Key, err = jwt.ParseRSAPublicKeyFromPEM([]byte(keyYeah))
	if err != nil {
		log.Error(err)
	}

	authorizer := auth.NewAuthorizer(MustMapEnv("AUTH0_SCOPE"), MustMapEnv("AUTH0_AUDIENCE"), MustMapEnv("AUTH0_ISSUER"), MustMapEnv("AUTH0_SUBJECT"), Key)
	// grpc server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", "3000"))
	if err != nil {
		log.Printf("failed to listen: %v", err)
	}

	s := grpc.NewServer(grpc.Creds(tlsCredentials), grpc.ChainUnaryInterceptor(authorizer.EnsureValidToken, nrgrpc.UnaryServerInterceptor(app)))
	pb.RegisterHelloServiceServer(s, &server{})

	if err := s.Serve(lis); err != nil {
		log.Printf("failed to serve: %s", err)
	}
}

func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloRequest, error) {
	fmt.Printf("Received: %v\n", in.GetName())

	hostname, _ := os.Hostname()
	app.RecordCustomMetric("SayHello", 1)
	return &pb.HelloRequest{Name: hostname}, nil
}

func MustMapEnv(key string) string {
	env := os.Getenv(key)
	if env == "" {
		panic(fmt.Sprintf("environment variable %s not set", key))
	}
	return env
}
