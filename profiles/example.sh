REQUIRES="kind kubectl"

create() {
  kind create cluster --name example --image kindest/node:v1.31.0
  echo "Example cluster created."
}

delete() {
  kind delete cluster --name example
  echo "Example cluster deleted."
}
