# grpc-go-vault
⚡️ gRPC server and client built with Go and secured with TLS certificates generated with HashiCorp Vault ⚡️


## Getting started

```
docker-compose up -d
cd terraform/
terraform init
terraform apply --auto-approve
cd ..
go run server.go
go run client.go
```