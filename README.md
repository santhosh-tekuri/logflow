# Logflow

```shell
docker build -t logflow:0.1.0 .
kubectl create ns logflow
kubectl apply -k kustomize
```

# Performance

select a worker node for peformance test and label it:

```shell
$ kubectl get nodes
NAME      STATUS   ROLES    AGE   VERSION
master    Ready    master   56d   v1.15.0
worker1   Ready    <none>   56d   v1.15.0
worker2   Ready    <none>   56d   v1.15.0

$ kubectl label nodes worker2 perf=true
node/worker2 labeled
```
here we chose `worker2` as perf node

install logflow and wait for pods:
```shell
$ kubectl apply -k perf/logflow
namespace/logflow created
serviceaccount/logflow created
clusterrole.rbac.authorization.k8s.io/logflow created
clusterrolebinding.rbac.authorization.k8s.io/logflow created
configmap/logflow-ff5k2b2t4d created
service/elasticsearch created
service/kibana created
deployment.apps/elasticsearch created
deployment.apps/kibana created
daemonset.apps/logflow created

$ kubectl -n logflow get po
NAME                            READY   STATUS    RESTARTS   AGE
elasticsearch-598959bf8-846l7   1/1     Running   0          17s
kibana-5d7ff5fd79-vl56h         1/1     Running   0          17s
logflow-pwn4s                   1/1     Running   0          17s
logflow-vh885                   1/1     Running   0          17s
logflow-xw8sn                   1/1     Running   0          17s
```

now run counter deployment as follows:

this deployment has container which produce log message per millisec.  
it has replica 10. thus this deployment produces 10k log messages per sec.  
all pods are launched on perf worker node using `nodeSelector`

```shell
$ kubectl apply -k perf/counter.yaml
deployment.apps/counter configured
```

to see cpu usage of logflow, ssh to perf worker node and run `top` command, press `o` and type `COMMAND=logflow`:  

```shell
Tasks: 123 total,   1 running,  76 sleeping,   0 stopped,   0 zombie
%Cpu(s):  0.7 us,  0.3 sy,  0.0 ni, 99.0 id,  0.0 wa,  0.0 hi,  0.0 si,  0.0 st
KiB Mem :  4039536 total,  2561628 free,   313312 used,  1164596 buff/cache
KiB Swap:        0 total,        0 free,        0 used.  3481144 avail Mem

  PID USER      PR  NI    VIRT    RES    SHR S  %CPU %MEM     TIME+ COMMAND
13951 root      20   0  108572   8236   4648 S   3.5  0.4   0:00.51 logflow
```

you can see that fluentd takes 3-5% cpu

let us brigdown the setup:

```shell
$ kubectl delete -f perf/counter.yaml
deployment.apps "counter" deleted

$ kubectl delete -k perf/logflow
namespace "logflow" deleted
serviceaccount "logflow" deleted
clusterrole.rbac.authorization.k8s.io "logflow" deleted
clusterrolebinding.rbac.authorization.k8s.io "logflow" deleted
configmap "logflow-ff5k2b2t4d" deleted
service "elasticsearch" deleted
service "kibana" deleted
deployment.apps "elasticsearch" deleted
deployment.apps "kibana" deleted
daemonset.apps "logflow" deleted
```

now install fluentd and wait for pods:

```shell script
$ kubectl apply -k perf/fluentd
namespace/fluentd created
serviceaccount/fluentd created
clusterrole.rbac.authorization.k8s.io/fluentd created
clusterrolebinding.rbac.authorization.k8s.io/fluentd created
configmap/fluentd-m82mm29m42 created
service/elasticsearch created
service/kibana created
deployment.apps/elasticsearch created
deployment.apps/kibana created
daemonset.apps/fluentd created

$ kubectl -n fluentd get po -wide
NAME                            READY   STATUS    RESTARTS   AGE   IP             NODE      NOMINATED NODE   READINESS GATES
elasticsearch-598959bf8-5hbxk   1/1     Running   0          87s   10.244.1.131   worker1   <none>           <none>
fluentd-9lrqt                   1/1     Running   0          87s   10.244.1.130   worker1   <none>           <none>
fluentd-g99xs                   1/1     Running   0          87s   10.244.0.78    master    <none>           <none>
fluentd-z87gs                   1/1     Running   0          87s   10.244.2.125   worker2   <none>           <none>
kibana-5d7ff5fd79-llcq9         1/1     Running   0          87s   10.244.1.129   worker1   <none>           <none>
```

run counter deployment:

```shell script
$ kubectl apply -f perf/counter.yaml
```

```shell script
$ kubectl -n fluentd exec -it fluentd-z87gs bash
root@fluentd-z87gs:/home/fluent# top
top - 13:40:16 up  2:28,  0 users,  load average: 6.47, 3.58, 4.34
Tasks:   5 total,   1 running,   4 sleeping,   0 stopped,   0 zombie
%Cpu(s):  4.5 us, 10.0 sy,  0.0 ni, 85.0 id,  0.2 wa,  0.0 hi,  0.3 si,  0.0 st
KiB Mem :  4039536 total,  2293152 free,   522056 used,  1224328 buff/cache
KiB Swap:        0 total,        0 free,        0 used.  3303532 avail Mem

  PID USER      PR  NI    VIRT    RES    SHR S  %CPU %MEM     TIME+ COMMAND
   11 root      20   0  263180 100556   9904 S  31.2  2.5   0:08.95 ruby
    1 root      20   0   15828   1840   1716 S   0.0  0.0   0:00.01 tini
    6 root      20   0  197328  52904   9256 S   0.0  1.3   0:01.44 ruby
   21 root      20   0   27944   3864   3364 S   0.0  0.1   0:00.00 bash
   28 root      20   0   50136   3940   3304 R   0.0  0.1   0:00.00 top
```

let us brigdown the setup:

```shell script
$ kubectl delete -f perf/counter.yaml
deployment.apps "counter" deleted

$ kubectl delete -k perf/fluentd
namespace "fluentd" deleted
serviceaccount "fluentd" deleted
clusterrole.rbac.authorization.k8s.io "fluentd" deleted
clusterrolebinding.rbac.authorization.k8s.io "fluentd" deleted
configmap "fluentd-m82mm29m42" deleted
service "elasticsearch" deleted
service "kibana" deleted
deployment.apps "elasticsearch" deleted
deployment.apps "kibana" deleted
daemonset.apps "fluentd" deleted

$ kubectl label nodes worker2 perf-
node/worker2 labeled
```

you can see that fluentd takes 30-40% cpu