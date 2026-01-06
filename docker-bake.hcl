
// variables
variable "OWNER_NAME" {
  type = string
  default = "feederbox826"
}

variable "IMAGE_NAME" {
  type = string
  default = "webp-watch-go"
}

// targets
target "alpine" {
  context = "."
  dockerfile = "Dockerfile"
  tags = [
    "ghcr.io/${OWNER_NAME}/${IMAGE_NAME}:latest",
    "ghcr.io/${OWNER_NAME}/${IMAGE_NAME}:amd64"
  ]
  cache-to = [{ type = "gha", mode = "max" }]
  cache-from = [{ type = "gha" }]
}

target "alpine-arm64" {
  inherits = ["alpine"]
  tags = [
    "ghcr.io/${OWNER_NAME}/${IMAGE_NAME}:arm64"
  ]
  platforms = ["linux/arm64"]
}

target "alpine-debug" {
  inherits = ["alpine"]
  tags = [
    "${IMAGE_NAME}"
  ]
  cache-from = [{ type="registry", ref="ghcr.io/${OWNER_NAME}/${IMAGE_NAME}:amd64" }]
}