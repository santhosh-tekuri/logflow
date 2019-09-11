# Logflow - deveopment in progress

Logflow exports kubernetes pod logs to Elasticsearch.  
This project goal is use minimum cpu(1 to 2%) and minimum memory, in comparison to other solutions.  
It is written in golang and is lightweight.

## How it works

- watches for any changes to log files in `/var/log/containers`. this directory contains symlinks
  to docker log files
- if new log file appears in `/var/log/containers`, resolves to its realpath
- it creates hardlink to the log file in `/var/log/containers/logflow` directory
- when docker rotates log file, it creates hardlink to new log file in `/var/log/containers/logflow`
- because we create hardlinks to log files, no additional disk space is required by logflow, 
  other than few metadata files in `/var/log/containers/logflow`
- a new goroutine is started for each pod, which parses the log files in `/var/log/containers/logflow` 
  and exports to elastic search.
- once a logfile completely exported, it is deleted from `/var/log/containers/logflow`
- thus if elasticsearch is reachable, logflow should use only diskspace only for small metadata files.
- in case, elasticsearch is down, we keep deleting old logfile from `/var/log/containers/logflow`
  when docker rotates new logfile. you can configure how many additional logfiles can be stored 
  other than what docker keeps on disk with `maxFiles` property in `logflow.conf` 
  
## Quickstart

Make sure that kubernetes nodes are using docker [json-file](https://docs.docker.com/config/containers/logging/json-file/) logging driver.
you can check this in `/etc/docker/daemon.json` file.

To use `json-file` logging driver create `/etc/docker/daemon.json` with below content and restart docker.

```json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3" 
  }
}
```

clone this project, and edit `kustomize/logflow.conf`
- update `elasticsearch.url`
- update `json-file.max-file` to same value as in `/etc/docker/daemon.json`
- leave other options to their defaults

now deploy logflow into namespace `logflow`:

```shell
$ kubectl create ns logflow
namespace/logflow created

$ kubectl apply -k kustomize
serviceaccount/logflow created
clusterrole.rbac.authorization.k8s.io/logflow created
clusterrolebinding.rbac.authorization.k8s.io/logflow created
configmap/logflow-ff5k2b2t4d created
service/elasticsearch created
service/kibana created
deployment.apps/elasticsearch created
deployment.apps/kibana created
daemonset.apps/logflow created
```

the logs are exported to elasticsearch indexes with format `logflow-yyyy-mm-dd`.  
all log records has 3 mandatory fields: `@timestamp`, `@message` and `@k8s`
- `@timestamp` is in RFC3339 Nano format
- `@message` is log message
- `@k8s` is json object with fields:
    - `namespace` namespace of pod
    - `pod` name of the pod
    - `container_name` name of the container
    - `container_id` docker container id
        - you can see a specific pod instance logs in kibana, by applying filter on this field
    - `nodename` name of node on which it is running
    - `labels` json object of labels
        - if label name contains `.` it is replaced with `_`

you can add additions fields such as loglevel, threadname etc to log record, by configuring log parsing as explained below. 


how to parser a pod logs, is specified by adding annotation `logflow.io/parser` on pod.

to parse log using [regex](https://github.com/google/re2/wiki/Syntax) format:
```yaml
annotations:
  logflow.io/parser: |-
    format=/^\[(?P<timestamp>.*?)\] (?P<message>.*)$/
    message_key=message
    timestamp_key=timestamp
    timestamp_layout=Mon Jan _2 15:04:05 MST 2006
    multiline_start=/^\[(?P<time>.*?)\] /
```
- regex must be enclosed in `/`

- you can test your regex [here](https://play.golang.org/p/J7NJr_nTskK)
    - edit `line` and `expr` in the opened page and click `Run`
- group names `?P<GROUPNAME>` in regex will map to log record field names
- `message_key` is mandatory. it allows to replace `@message` value in log record with the specified regex group match
- `timestamp_key` allows to replace `@timestamp` value in log record with the specified regex group match
    - `timestamp_layout` specified time format based on reference time "Mon Jan 2 15:04:05 -0700 MST 2006"
    - see [this](https://medium.com/@simplyianm/how-go-solves-date-and-time-formatting-8a932117c41c) to understand time_layout format
- `multiline_start` is regexp pattern for start line of multiple lines. this is useful if log message can extend to more than one line.
   the loglines which do not match this regexp are treated as part of recent log message. note that regexp in `format` is matched only 
   on the first line, not on complete multiline log message.


to parse log using json format:
```yaml
annotations:
  logflow.io/parser: |-
    format=json
    message_key=message
    timestamp_key=time
    timestamp_layout=Mon Jan _2 15:04:05 MST 2006
```

- `message_key` is mandatory. it allows to replace `@message` value in log record with the specified json field value
- `timestamp_key` allows to replace `@timestamp` value in log record with the specified json field value
    - `timestamp_layout` specified time format based on reference time "Mon Jan 2 15:04:05 -0700 MST 2006"
    - see [this](https://medium.com/@simplyianm/how-go-solves-date-and-time-formatting-8a932117c41c) to understand time_layout format
- top level non-string fields are suffixed with their json type. consider an example where one pod log has
  `error` field with string value and another pod log has `error` field with object having more details. in 
  such cases, elasticsearch throws `mapper_parsing_exception`. to avoid this, logflow renames the `error` field
  with object value to `error$obj`. this avoids mapping exceptions to large extent without additional manual 
  configuration

to exclude logs of a pod:
```yaml
annotations:
  logflow.io/exclude: "true"
```

Note that the annotation value is boolean which can take a `true` or `false` and must be quoted.

If pod has multiple containers with different log format use `logflow.io/parser-CONTAINER` annotation
to target specific container. For example to target container named `nginx`, use annotation `logflow.io/parser-nginx`

similarly to exclude logs from specific container use `logflow.io/exclude-CONTAINER` annotation

NOTE:

- logflow does not watch for changes to annotation `logflow.io/parser`
- logflow reads this annotation only when pod is deployed
- so any changes to this annotation, after pod is deployed are not reflected

## Performance

As per my tests, for 10k messages per second:
- logflow takes 1 to 2% cpu
- fluentd takes 30 to 40% cpu
- fluent-bit takes 4-5% cpu

Below are the instructions to run performance tests to compare logflow with fluentd.  

Make sure that you have minimum 4G memory on each kubernetes node. 
because we are deploying elasticsearch and kibana.

Make sure that kubernetes nodes are using docker [json-file](https://docs.docker.com/config/containers/logging/json-file/) logging driver.
you can check this in `/etc/docker/daemon.json` file.

To use `json-file` logging driver create `/etc/docker/daemon.json` with below content and restart docker.

```json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3" 
  }
}
```

clone this project

select a worker node say `worker2` for peformance test and `perf` label:

```shell
$ kubectl get nodes
NAME      STATUS   ROLES    AGE   VERSION
master    Ready    master   56d   v1.15.0
worker1   Ready    <none>   56d   v1.15.0
worker2   Ready    <none>   56d   v1.15.0

$ kubectl label nodes worker2 perf=true
node/worker2 labeled
```

### logflow

edit `kustomize/logflow.conf`
- update `json-file.max-file` to same value as in `/etc/docker/daemon.json`
- leave other options to their defaults

install `logflow` and wait for pods:

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

this deployment has container which produces one log message per millisec.  
it has replica 10. thus this deployment produces 10k log messages per sec.  
all pods are launched on perf worker node using `nodeSelector`

```shell
$ kubectl apply -f perf/counter.yaml
deployment.apps/counter created
```

to see cpu usage of logflow, ssh to perf worker node and run `top` command, press `o` and type `COMMAND=logflow`:  

```shell
Tasks: 123 total,   1 running,  76 sleeping,   0 stopped,   0 zombie
%Cpu(s):  0.7 us,  0.3 sy,  0.0 ni, 99.0 id,  0.0 wa,  0.0 hi,  0.0 si,  0.0 st
KiB Mem :  4039536 total,  2561628 free,   313312 used,  1164596 buff/cache
KiB Swap:        0 total,        0 free,        0 used.  3481144 avail Mem

  PID USER      PR  NI    VIRT    RES    SHR S  %CPU %MEM     TIME+ COMMAND
13951 root      20   0  108572   8236   4648 S   1.3  0.4   0:00.51 logflow
```

you can see that `logflow` takes 1-2% cpu

NOTE: at startup it may take more cpu than above because of old logs, but after some time cpu will be low. 

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

### fluentd

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
deployment.apps/counter created
```

bash into fluentd pod which is running on perf node, to see cpu usage:

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

you can see that `fluentd` takes 30-40% cpu

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
```

### fluent-bit

now install fluent-bit and wait for pods:

```shell script
$ kubectl apply -k perf/fluent-bit/
namespace/fluent-bit created
serviceaccount/fluent-bit created
clusterrole.rbac.authorization.k8s.io/fluent-bit-read created
clusterrolebinding.rbac.authorization.k8s.io/fluent-bit-read created
configmap/fluent-bit-config-8m7b4c5kth created
service/elasticsearch created
service/kibana created
deployment.apps/elasticsearch created
deployment.apps/kibana created
daemonset.extensions/fluent-bit created
```

run counter deployment:

```shell script
$ kubectl apply -f perf/counter.yaml
deployment.apps/counter created
```

to see cpu usage of fluent-bit, ssh to perf worker node and run `top` command, press `o` and type `COMMAND=fluent-bit`:

```shell script
$ top
top - 13:32:17 up 16 min,  3 users,  load average: 11.36, 7.43, 3.41
Tasks: 187 total,   8 running, 125 sleeping,   0 stopped,   2 zombie
%Cpu(s): 29.0 us, 62.3 sy,  0.0 ni,  4.6 id,  2.0 wa,  0.0 hi,  2.1 si,  0.0 st
KiB Mem :  4039644 total,   618804 free,  2076472 used,  1344368 buff/cache
KiB Swap:        0 total,        0 free,        0 used.  1704044 avail Mem

  PID USER      PR  NI    VIRT    RES    SHR S  %CPU %MEM     TIME+ COMMAND
 8414 root      20   0  144884  10784   6580 R   4.3  0.3   0:11.85 fluent-bit
```

you can see that `fluent-bit` takes 4-5% cpu

let us brigdown the setup:

```shell script
$ kubectl delete -f perf/counter.yaml
deployment.apps "counter" deleted

$ kubectl delete -k perf/fluent-bit
namespace "fluent-bit" deleted
serviceaccount "fluent-bit" deleted
clusterrole.rbac.authorization.k8s.io "fluent-bit-read" deleted
clusterrolebinding.rbac.authorization.k8s.io "fluent-bit-read" deleted
configmap "fluent-bit-config-8m7b4c5kth" deleted
service "elasticsearch" deleted
service "kibana" deleted
deployment.apps "elasticsearch" deleted
deployment.apps "kibana" deleted
daemonset.extensions "fluent-bit" deleted

$ kubectl label nodes worker2 perf-
node/worker2 labeled
```