terraform { backend "s3" { bucket = "platform-tfstate" key = "envs/prod/terraform.tfstate" region = "eu-central-1" } }

module "hetzner" {
  source             = "../../modules/hetzner"
  env                = "prod"
  node_type          = "cax31"
  ssh_pubkey         = file("~/.ssh/id_ed25519.pub")
  cloudflare_zone_id = var.cloudflare_zone_id
  domain             = "example.com"
}
variable "cloudflare_zone_id" { type = string }
