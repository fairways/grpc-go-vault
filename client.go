package main

import (
	"context"
	"fmt"
	"google.golang.org/grpc/credentials/oauth"
	"log"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/certutil"
	pb "github.com/jamiewhitney/grpc-go-vault/hello"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

	tlsConfig, err := parsedCertBundle.GetTLSConfig(certutil.TLSClient)
	if err != nil {
		fmt.Errorf("Could not get TLS config: %s", err)
	}

	tlsCredentials := credentials.NewTLS(tlsConfig)

	//grpc

	x := "Jamie"
	perRPC := oauth.NewOauthAccess(fetchToken())
	conn, err := grpc.Dial(":3000", grpc.WithTransportCredentials(tlsCredentials), grpc.WithPerRPCCredentials(perRPC))
	if err != nil {
		log.Fatalf("did not connect: %s", err)
	}
	defer conn.Close()

	client := pb.NewHelloServiceClient(conn)

	for {
		response, err := client.SayHello(context.Background(), &pb.HelloRequest{Name: x})
		if err != nil {
			log.Fatalf("Error when calling SayHello: %s", err)
		}
		log.Printf("Response from Server: %s", response.GetName())
		time.Sleep(2 * time.Second)
	}
}

func fetchToken() *oauth2.Token {
	return &oauth2.Token{
		AccessToken: "some-secret-token2",
	}
}
