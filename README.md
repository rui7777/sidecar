# sidecar

```
$ eval $(minikube docker-env)
$ docker build -t localimage/gameserver:latest .
$ kubectl port-forward svc/autoscaler-webhook-service 8000:8000
```

```
$ brew install kustomize
$ cd k8s/base
$ kustomize edit set image localimage/gameserver:latest
$ kustomize build .
```