#! /bin/sh

kubectl delete -f ./k8s/sidecar.yaml

docker images localimage/game-server -q && docker images -f "dangling=true" -q

docker build -t localimage/game-server:latest .

kubectl create -f ./k8s/sidecar.yaml
