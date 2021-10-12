# sidecar

```
$ eval $(minikube docker-env)
$ docker build -t localimage/server1:latest ./server1
$ docker build -t localimage/server2:latest ./server2
$ kubectl create -f ./pod.yaml
$ kubectl describe service gameserver
$ kubectl exec -it gameserver /bin/ash
```