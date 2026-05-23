terraform { backend "s3" { bucket = "platform-tfstate" key = "envs/staging/terraform.tfstate" region = "eu-central-1" } }

module "hetzner" {
  source             = "../../modules/hetzner"
  env                = "staging"
  ssh_pubkey         = file("~/.ssh/id_ed25519.pub")
  cloudflare_zone_id = var.cloudflare_zone_id
  domain             = "staging.example.com"
}
variable "cloudflare_zone_id" { type = string }
