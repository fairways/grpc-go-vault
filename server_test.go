package main

import (
	"context"
	"fmt"
	pb "github.com/jamiewhitney/grpc-go-vault/hello"
	"google.golang.org/grpc"
	"testing"
)

var (
	port = "3000"
)

type server struct {
	pb.UnimplementedHelloServiceServer
}

func TestMessages(t *testing.T) {

	// Set up a connection to the Server.
	conn, err := grpc.Dial(fmt.Sprintf("localhost:%s", port), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewHelloServiceClient(conn)

	// Test SayHello

	// set up test cases
	tests := []struct {
		name string
		want string
	}{
		{
			name: "world",
			want: "Hello world",
		},
		{
			name: "123",
			want: "Hello 123",
		},
	}

	for _, tt := range tests {
		req := &pb.HelloRequest{Name: tt.name}
		resp, err := c.SayHello(context.Background(), req)
		if err != nil {
			t.Errorf("HelloTest(%v) got unexpected error", err)
		}
		if resp.Name != tt.want {
			t.Errorf("HelloText(%v)=%v, wanted %v", tt.name, resp.Name, tt.want)
		}
	}
}
