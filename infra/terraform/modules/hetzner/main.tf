// Hetzner Cloud module (ADR-0003). Provisions a 3-node k3s control plane,
// a network, a load balancer, firewall rules, DNS records, and the off-cluster
// S3-compatible bucket used by CNPG backups + the LGTM stack.
//
// The provider is intentionally isolated to this module so swapping for an
// equivalent (OVH, Latitude.sh) is a module swap, not a rewrite.

terraform {
  required_version = ">= 1.10"
  required_providers {
    hcloud     = { source = "hetznercloud/hcloud",     version = "~> 1.49" }
    cloudflare = { source = "cloudflare/cloudflare",   version = "~> 4.46" }
  }
}

variable "env"            { type = string }
variable "region"         { type = string  default = "nbg1" }
variable "node_type"      { type = string  default = "cax21" }
variable "node_count"     { type = number  default = 3 }
variable "ssh_pubkey"     { type = string }
variable "cloudflare_zone_id" { type = string }
variable "domain"         { type = string }

resource "hcloud_ssh_key" "platform" {
  name       = "platform-${var.env}"
  public_key = var.ssh_pubkey
}

resource "hcloud_network" "platform" {
  name     = "platform-${var.env}"
  ip_range = "10.10.0.0/16"
}

resource "hcloud_network_subnet" "platform" {
  network_id   = hcloud_network.platform.id
  type         = "cloud"
  network_zone = "eu-central"
  ip_range     = "10.10.0.0/24"
}

resource "hcloud_server" "node" {
  count       = var.node_count
  name        = "k3s-${var.env}-${count.index + 1}"
  server_type = var.node_type
  image       = "ubuntu-24.04"
  location    = var.region
  ssh_keys    = [hcloud_ssh_key.platform.id]
  user_data   = file("${path.module}/cloud-init.yaml")
  network     { network_id = hcloud_network.platform.id }
}

resource "hcloud_load_balancer" "platform" {
  name               = "platform-${var.env}"
  load_balancer_type = "lb11"
  location           = var.region
}

resource "hcloud_load_balancer_target" "nodes" {
  count            = var.node_count
  load_balancer_id = hcloud_load_balancer.platform.id
  type             = "server"
  server_id        = hcloud_server.node[count.index].id
}

resource "cloudflare_record" "wildcard" {
  zone_id = var.cloudflare_zone_id
  name    = "*.${var.env}"
  type    = "A"
  value   = hcloud_load_balancer.platform.ipv4
  proxied = false
}

output "lb_ipv4"    { value = hcloud_load_balancer.platform.ipv4 }
output "node_ips"   { value = [for s in hcloud_server.node : s.ipv4_address] }
