# sidecar

```
$ eval $(minikube docker-env)
$ docker build -t localimage/gameserver:latest .
$ kubectl port-forward svc/autoscaler-webhook-service 8000:8000
```