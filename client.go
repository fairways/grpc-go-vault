package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc/credentials/oauth"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/certutil"
	pb "github.com/jamiewhitney/grpc-go-vault/hello"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"math/rand"
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

	// token
	auth0ClientId, err := vaultClient.Logical().Read("hello-service/data/auth0")
	if err != nil {
		fmt.Errorf("failed to retrieve token")
	}

	authTokenData := auth0ClientId.Data["data"].(map[string]interface{})

	clientToken := authTokenData["id"].(string)

	clientSecret := authTokenData["secret"].(string)

	url := authTokenData["domain"].(string)

	audience := authTokenData["audience"].(string)

	//grpc

	x := "Jamie"
	perRPC := oauth.NewOauthAccess(fetchToken(clientToken, clientSecret, url, audience, "client_credentials"))
	fmt.Println("got the token boy")
	fmt.Printf("%+v", perRPC)
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
		n := rand.Intn(10)
		time.Sleep(time.Duration(n) * time.Second)
	}
}

func fetchToken(id string, secret string, url string, audience string, grantType string) *oauth2.Token {
	var tokenObject TokenReponse

	data := TokenRequest{
		ClientId:     id,
		ClientSecret: secret,
		Audience:     audience,
		GrantType:    grantType,
	}

	payload, _ := json.Marshal(data)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))

	req.Header.Add("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err)
	}

	defer res.Body.Close()
	responseData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
	}

	json.Unmarshal(responseData, &tokenObject)
	fmt.Printf("%s", responseData)
	return &oauth2.Token{
		AccessToken: tokenObject.AccessToken,
	}
}
