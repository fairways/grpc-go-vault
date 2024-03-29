package main

import (
	"context"
	"fmt"
	"github.com/jamiewhitney/auth-jwt-grpc"
	"os"

	vault "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/certutil"
	pb "github.com/jamiewhitney/grpc-go-vault/hello"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"log"
	"time"
)

type TokenReponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type TokenRequest struct {
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Audience     string `json:"audience"`
	GrantType    string `json:"grant_type"`
}

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
		fmt.Errorf("Error parsing secret: %s", err)
	}

	tlsConfig, err := parsedCertBundle.GetTLSConfig(certutil.TLSClient)
	if err != nil {
		fmt.Errorf("Could not get TLS config: %s", err)
	}

	tlsCredentials := credentials.NewTLS(tlsConfig)

	// token
	auth0ClientId, err := vaultClient.Logical().Read("hello-service/data/auth0")
	if err != nil {
		fmt.Errorf("failed to retrieve token")
	}

	authTokenData := auth0ClientId.Data["data"].(map[string]interface{})

	clientToken := authTokenData["id"].(string)

	clientSecret := authTokenData["secret"].(string)

	url := authTokenData["url"].(string)

	audience := authTokenData["audience"].(string)

	// grpc
	perRPC := oauth.NewOauthAccess(auth.FetchToken(clientToken, clientSecret, url, audience, "client_credentials"))

	conn, err := grpc.Dial(":3000", grpc.WithTransportCredentials(tlsCredentials), grpc.WithPerRPCCredentials(perRPC))
	if err != nil {
		log.Fatalf("did not connect: %s", err)
	}
	defer conn.Close()

	client := pb.NewHelloServiceClient(conn)

	for {
		response, err := client.SayHello(context.Background(), &pb.HelloRequest{Name: "Jamie"})
		if err != nil {
			log.Fatalf("error when calling SayHello: %s", err)
		}
		log.Printf("Response from Server: %s", response.GetName())
		time.Sleep(time.Duration(1) * time.Second)
	}
}
