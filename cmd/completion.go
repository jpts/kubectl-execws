package cmd

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MainValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	s, cerr := initCompletionCliSession()
	if cerr != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeAvailablePods(s, toComplete)
}

func NamespaceValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	s, cerr := initCompletionCliSession()
	if cerr != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return completeAvailableNS(s, toComplete)
}

func ContainerValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	s, cerr := initCompletionCliSession()
	if cerr != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	s.opts.Pod = args[0]
	return completeAvailableContainers(s, toComplete)
}

func initCompletionCliSession() (*cliSession, error) {
	return NewCliSession(&cliopts)
}

func completeAvailableNS(c *cliSession, toComplete string) ([]string, cobra.ShellCompDirective) {
	res, err := c.k8sClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var nspaces []string
	for _, ns := range res.Items {
		if strings.HasPrefix(ns.Name, toComplete) {
			nspaces = append(nspaces, ns.Name)
		}
	}

	return nspaces, cobra.ShellCompDirectiveNoFileComp
}

func completeAvailablePods(c *cliSession, toComplete string) ([]string, cobra.ShellCompDirective) {
	res, err := c.k8sClient.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var pods []string
	for _, pod := range res.Items {
		if strings.HasPrefix(pod.Name, toComplete) {
			//noCtrs := len(pod.Spec.Containers)
			//noEphem := len(pod.Spec.EphemeralContainers)
			pods = append(pods, pod.Name)
		}
	}

	return pods, cobra.ShellCompDirectiveNoFileComp
}

func completeAvailableContainers(c *cliSession, toComplete string) ([]string, cobra.ShellCompDirective) {
	res, err := c.k8sClient.CoreV1().Pods(c.namespace).Get(context.TODO(), c.opts.Pod, metav1.GetOptions{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var ctrs []string
	for _, ctr := range res.Spec.Containers {
		if strings.HasPrefix(ctr.Name, toComplete) {
			ctrs = append(ctrs, ctr.Name)
		}
	}
	for _, ctr := range res.Spec.EphemeralContainers {
		if strings.HasPrefix(ctr.Name, toComplete) {
			ctrs = append(ctrs, ctr.Name)
		}
	}

	return ctrs, cobra.ShellCompDirectiveNoFileComp
}
