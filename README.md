# kubectl-execws

A replacement for "kubectl exec" that works over WebSocket connections.

Kubernetes API server has support for exec over WebSockets, but it has yet to land in kubectl. This plugin is designed to be a stopgap until then!

Usage:
```
A replacement for "kubectl exec" that works over WebSocket connections.

Usage:
  execws <pod name> [--kubeconfig] [-n namespace] [-it] [-c container] <cmd> [flags]

Flags:
  -c, --container string    Container name
  -h, --help                help for execws
      --kubeconfig string   kubeconfig file (default is $HOME/.kube/config)
  -n, --namespace string    Override "default" namespace
  -i, --stdin               Pass stdin to container
  -t, --tty                 Stdin is a TTY
```

### ToDo
* raw terminal mode
* correctly handle signals

### Acknowledgements

Work inspired by [rmohr/kubernetes-custom-exec](https://github.com/rmohr/kubernetes-custom-exec) and [kairen/websocket-exec](https://github.com/kairen/websocket-exec).
