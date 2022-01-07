# kubectl-execws

A replacement for "kubectl exec" that works over WebSocket connections.

Kubernetes API server has support for exec over WebSockets, but it has yet to land in kubectl. This plugin is designed to be a stopgap until then!

Usage:
```
execws <pod name> [--kubeconfig] [-n namespace] [-it] [-c container] <cmd>
```

### Acknowledgements

Work inspired by [rmohr/kubernetes-custom-exec](https://github.com/rmohr/kubernetes-custom-exec) and [kairen/websocket-exec](https://github.com/kairen/websocket-exec).
