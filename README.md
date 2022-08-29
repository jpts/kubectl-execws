# kubectl-execws

A replacement for "kubectl exec" that works over WebSocket connections.

The Kubernetes API server has support for exec over WebSockets, but it has yet to land in kubectl. Although [some](https://github.com/kubernetes/kubernetes/issues/89163) [proposals](https://github.com/kubernetes/enhancements/pull/3401) exist to add the functionality, they seem quite far away from landing. This plugin is designed to be a stopgap until they do.

Usage:
```
Usage:
  execws <pod name> [options] <cmd>

Flags:
  -c, --container string             Container name
  -h, --help                         help for execws
      --kubeconfig string            kubeconfig file (default is $HOME/.kube/config)
  -v, --loglevel int                 Set loglevel (default 2)
  -n, --namespace string             Set namespace
      --no-sanity-check              Don't make preflight request to ensure pod exists
      --node-direct-exec             Partially bypass the API server, by using the kubelet API
      --node-direct-exec-ip string   Node IP to use with direct-exec feature
  -k, --skip-tls-verify              Don't perform TLS certificate verifiation
  -i, --stdin                        Pass stdin to container
  -t, --tty                          Stdin is a TTY
```

## Features

* Aware of `HTTP_PROXY`/`HTTPS_PROXY` env variables
* Uses standard Kubeconfig processing including `~/.kube/config` & `$KUBECONFIG` support
* Doesn't use SPDY so might be more loadbalancer/reverse proxy friendly
* Supports a full TTY (terminal raw mode)
* Can bypass the API server with direct connection to the nodes kubelet API

### Acknowledgements

Work inspired by [rmohr/kubernetes-custom-exec](https://github.com/rmohr/kubernetes-custom-exec) and [kairen/websocket-exec](https://github.com/kairen/websocket-exec).
