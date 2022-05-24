package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"net"
	"strings"

	vault "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/certutil"
	pb "github.com/jamiewhitney/grpc-go-vault/hello"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type server struct {
	pb.UnimplementedHelloServiceServer
}

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
	authToken          string
)

func main() {

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

	// token
	authTokenSecret, err := vaultClient.Logical().Read("hello-service/data/token")
	if err != nil {
		fmt.Errorf("failed to retrieve token")
	}

	authTokenData := authTokenSecret.Data["data"].(map[string]interface{})

	authToken = authTokenData["token"].(string)

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
	return &pb.HelloRequest{Name: "Hello " + in.GetName()}, nil
}

func valid(authorization []string) bool {
	if len(authorization) < 1 {
		return false
	}
	token := strings.TrimPrefix(authorization[0], "Bearer ")
	return token == authToken
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
