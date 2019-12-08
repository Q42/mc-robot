# Architecture
Date: 2019-12-06

The architecture of this app is a in-cluster container (Pod) that connects to PubSub to retrieve configuration from peers (other clusters).
Kubernetes Services will be created and maintained for each of the services exported by the peers.
Each instance reports the nodes of the cluster and the NodePorts that are a property of the services in the cluster to a central PubSub topic.

## Choice of ServiceType
Kubernetes has 4 service types:
1. ClusterIP: assigns a cluster internal IP address that each kube-proxy Round Robin-routes to pods
2. NodePort: this opens up ports at the nodes which route to a new ClusterIP service 
3. LoadBalancer: this provisions a LoadBalancer that points to a NodePort service
4. ExternalName: not an exposed service by a consumed one (e.g. just a CNAME to for example google.com)

As you can see, 3 is just an extension of 2 and 2 is an extension of 1. This are all the choices that we have for exposing the service.
Simply said, from the perpective of Kubernetes, if we want external traffic to enter the cluster we need at least 2 and it is up to us to
decide whether we let GKE provision a Load Balancer or not. Do we want a Load Balancer provisioned for us, and which?

## Load Balancers
We would have liked a solution where a cluster itself configures a set of LoadBalancers which are then accessible by other clusters using a single IP address. This separates the concerns in a certain way because each cluster is responsible for configuring its own LoadBalancers. Unfortunately, we've come to learn that Internal Load Balancers (Network LoadBalancers) are regional. This means that only traffic from the same region ends up in the Load Balancer, and machines in other regions can not route traffic to the Load Balancer: the traffic is simply dropped. Routing traffic from other regions to these ILBs is simply not possible, because you also can not configure a Global Load Balancer to have other Load Balancers (e.g. the other regions internal ones) as a backend (see Ref 1).

The alternative of just using Global Load Balancers is not acceptable because it requires opening up the service to the public internet (assigns a public IP address), whereas Internal Load Balancers use internal IP addresses (10.x.x.x) which are not public by default.

#### Ref 1
Pointing the GLB to multiple ILBs is impossible unfortunately:
> [StackOverflow](https://stackoverflow.com/a/58533550/552203):
> What you are trying to achieve is not possible. For 2 reasons.
> 1. GCP does not have GKE Ingress that can handle two different clusters. This is called multi cluster ingress and is not supported.
> 2. GCP load balancer can not have another load balancer as a backend.
