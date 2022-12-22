package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

func (c *cliSession) prepKubeletExec() (*http.Request, error) {
	var ip string
	if c.opts.directExecNodeIp != "" {
		ip = c.opts.directExecNodeIp
	} else {

		client, err := kubernetes.NewForConfig(c.clientConf)
		if err != nil {
			return nil, err
		}

		res, err := client.CoreV1().Nodes().Get(context.TODO(), c.opts.PodSpec.NodeName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		for _, addr := range res.Status.Addresses {
			if addr.Type == "InternalIP" {
				ip = addr.Address
			}
		}

		if ip == "" {
			return nil, errors.New("Unable to find Node IP")
		}
	}

	u, err := url.Parse(fmt.Sprintf("wss://%s:10250", ip))
	if err != nil {
		return nil, err
	}

	var ctrName string
	if c.opts.Container != "" {
		ctrName = c.opts.Container
	} else if len(c.opts.PodSpec.Containers) == 1 {
		ctrName = c.opts.PodSpec.Containers[0].Name
		klog.V(4).Infof("Discovered container name: %s", ctrName)
	} else {
		return nil, errors.New("Cannot determine container name")
	}

	u.Path = fmt.Sprintf("/exec/%s/%s/%s", c.namespace, c.opts.Pod, ctrName)
	query := url.Values{}
	query.Add("output", "1")
	query.Add("error", "1")

	for _, c := range c.opts.Command {
		query.Add("command", c)
	}

	if c.opts.TTY {
		query.Add("tty", "1")
	}

	if c.opts.Stdin {
		query.Add("input", "1")
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	klog.V(7).Infof("Making request to kubelet API:  %s:10250%s", ip, u.RequestURI())

	return req, nil

}
