terraform {
  backend "s3" {
    bucket = "platform-tfstate"
    key    = "envs/dev/terraform.tfstate"
    region = "eu-central-1"
    # endpoint set via env var for the Hetzner-S3-compatible bucket.
  }
}

module "hetzner" {
  source             = "../../modules/hetzner"
  env                = "dev"
  ssh_pubkey         = file("~/.ssh/id_ed25519.pub")
  cloudflare_zone_id = var.cloudflare_zone_id
  domain             = "dev.example.com"
}

variable "cloudflare_zone_id" { type = string }
output "lb_ipv4"  { value = module.hetzner.lb_ipv4 }
output "node_ips" { value = module.hetzner.node_ips }
