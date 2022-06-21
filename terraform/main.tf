terraform {
  required_providers {
    vault = {
      source = "hashicorp/vault"
      version = "3.2.1"
    }
  }
}

provider "vault" {
    address = "http://localhost:8200"
    token = "root"
  
}

resource "vault_mount" "pki" {
  path        = "grpc"
  type        = "pki"
  description = "PKI mount for gRPC certificates"
}

resource "vault_pki_secret_backend_root_cert" "main" {
  depends_on            = [vault_mount.pki]
  backend               = vault_mount.pki.path
  type                  = "internal"
  common_name           = "example.com"
  ttl                   = "315360000"
  format                = "pem"
  private_key_format    = "der"
  key_type              = "rsa"
  key_bits              = 4096
  exclude_cn_from_sans  = true
  ou                    = "example org"
  organization          = "example.com"
}

resource "vault_pki_secret_backend_role" "role" {
  backend          = vault_mount.pki.path
  name             = "hello-service"
  ttl              = "1800"
  allow_ip_sans    = true
  key_type         = "rsa"
  key_bits         = 4096
  allowed_domains  = ["example.com"]
  allow_subdomains = true

}

resource "vault_mount" "hello-service" {
  path        = "hello-service"
  type        = "kv-v2"
}

resource "vault_generic_secret" "hello-service-token" {
  path = "${vault_mount.hello-service.path}/auth0"
  data_json = <<EOT
{
 "id": "${var.auth0_id}",
 "secret": "${var.auth0_secret}"
 "public_cert": "${var.auth0_public_cert}"
 "url": "${var.auth0_url}"
 "audience: "${var.auth0_audience}"
}
EOT
}

variable "auth0_id" {
  type=string
}

variable "auth0_secret" {
  type=string
}

variable "auth0_public_cert" {
  type=string
}

variable "auth0_url" {
  type=string
}

variable "auth0_audience" {
  type=string
}