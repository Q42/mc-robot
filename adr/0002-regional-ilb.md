# Global Access for Regional ILB
Date: 2020-01-14

The previous ADR discussed unavailability of Internal Load Balancers in other
regions. However, [since December 2019](https://cloud.google.com/blog/products/networking/new-features-for-l4-internal-load-balancer) it has become possible to configure
_Global Access_ to these Regional Internal Load Balancers:

- in GCE (Beta):
	https://cloud.google.com/load-balancing/docs/internal/setting-up-internal#ilb-global-access

- and in GKE (1.17):
	https://cloud.google.com/kubernetes-engine/docs/how-to/internal-load-balancing#global_access_beta
	via `networking.gke.io/internal-load-balancer-allow-global-access: "true"`; see the [commit implementing this in GKE](https://github.com/kubernetes/legacy-cloud-providers/commit/85237499827ec0b56211296cdc5b9fda8efb659a).

This replaces some of the functionality of MC Robot. Previously we did not
want to use load balancer IPs as those would be either globally accessible
(not desired) or internal (not usable). If a Load Balancer is configured and
`EndpointsPublishPreferLoadBalancerIPs` is set, we can now use it because
either:
- the user configured Global Access and the IP address is internal but
  reachable in the whole network
- the user does not mind public external access and configured a regular Global
  Load Balancer for the service

If a Load Balancer is configured and `EndpointsPublishPreferLoadBalancerIPs`
is enabled, MC Robot will no longer export individual nodes but instead publish
the `Service#status.loadBalancer.ingress` field.

The use case for MC Robot remains, as it provides a way to automatically
provision new clusters based on only a shared PubSub topic.
