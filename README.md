docker build -t logflow:0.1.0 .
kubectl create ns logflow
kubectl apply -k kustomize
