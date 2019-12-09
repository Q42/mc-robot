# MC Robot <img src=".robot.jpeg#" height="100" />
An application that manages multi-cluster service discovery & setup.
The goal of this application is to make services in other clusters easily reachable.

The actions that are performed:
1. retrieve a list of k8s nodes
2. retrieve a list of k8s Services with a NodePort
3. publish list of nodes & services via PubSub mechanism
4. configure custom k8s Service & Endpoints pointing to services in other clusters

This meets the goals because after this is done, you can reach other clusters with
```bash
curl http://my-service-gke_my-project_europe-west4_my-cluster.default.svc.cluster.local
```

## 1. Usage
First install the CRD. Then build the operator & deploy it:
```bash
# CRD
kubectl apply -f deploy/crds/mc.q42.nl_servicesyncs_crd.yaml

# Build operator
export REGISTRY=quay.io/<user> # or gcr.io/project
operator-sdk build $REGISTRY/mc-robot:v1.0.0
sed -i "s|REPLACE_IMAGE|$REGISTRY/mc-robot:v1.0.0|g" deploy/operator.yaml
docker push $REGISTRY/mc-robot:v1.0.0

# Deploy operator
kubectl create -f deploy/service_account.yaml
kubectl create -f deploy/role.yaml
kubectl create -f deploy/role_binding.yaml
kubectl create -f deploy/operator.yaml
```

Create a ServiceSync object like this:
```yaml
apiVersion: mc.q42.nl/v1
kind: ServiceSync
metadata:
  name: example-servicesync
spec:
  topicURL: "gcppubsub://projects/myproject/topics/mytopic"
  selector:
    matchLabels:
      app: my-app
  endpointsPublishMax: 10
```

A topic url like `gcppubsub://projects/myproject/topics/mytopic` must be set.
The service sync controller must have access to this topic, which can be
configured through Application Default Crecentials with an environment variable
`GOOGLE_APPLICATION_CREDENTIALS` which should point to a file with a service
account, and which has access to that topic
([reference](https://gocloud.dev/howto/pubsub/publish/)).

## 2. Developing

### 2.1 Testing locally
```bash
$ OPERATOR_NAME=mc-robot operator-sdk up local --namespace=default
```

### 2.2 Generated code
A lot of the code for MC Robot is generated by the [operator-sdk](https://github.com/operator-framework/) framework.
Commands that were run:

```bash
$ brew install operator-sdk
$ export GO111MODULE=on
$ operator-sdk new mc-robot
$ cd mc-robot
$ operator-sdk add api --api-version=mc.q42.nl/v1 --kind=ServiceSync
$ operator-sdk add api --api-version=mc.q42.nl/v1 --kind=ServiceSync
$ operator-sdk generate k8s && operator-sdk generate openapi
$ operator-sdk add controller --api-version=mc.q42.nl/v1 --kind=ServiceSync
```

### 2.3 ADRs
- [Initial architecture](./adr/0001-architecture.md)

### 2.4 Working documentation
- https://medium.com/faun/writing-your-first-kubernetes-operator-8f3df4453234
- [How to define, build and run a CRD/controller](https://github.com/operator-framework/getting-started#define-the-memcached-spec-and-status)
- [How to write a reconciler](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/client.md)
