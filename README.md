# kubectl-execws

A replacement for "kubectl exec" that works over WebSocket connections.

Kubernetes API server has support for exec over WebSockets, but it has yet to land in kubectl. This plugin is designed to be a stopgap until then!

Usage:
```
execws <pod name> [--kubeconfig] [-n namespace] [-it] [-c container] <cmd>
```
## Features

* Aware of `HTTP_PROXY`/`HTTPS_PROXY` env variables
* Uses standard Kubeconfig processing including `~/.kube/config` & `$KUBECONFIG` support
* Doesn't use SPDY so might be more loadbalancer/reverse proxy friendly

### Acknowledgements

Work inspired by [rmohr/kubernetes-custom-exec](https://github.com/rmohr/kubernetes-custom-exec) and [kairen/websocket-exec](https://github.com/kairen/websocket-exec).
