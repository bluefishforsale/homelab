# kubelet
CreateContainerConfigError: failed to sync secret cache: timed out waiting for the condition
Aug 02 01:09:52 node003 kubelet[97022]: E0802 01:09:52.790937   97022 reflector.go:138] object-"metallb"/"metallb-1627865247-memberlist": Failed to watch *v1.Secret: failed to list *v1.Secret: Internal error occurred: invalid padding on input
Aug 02 16:31:06 node001 kubelet[210951]: E0802 16:31:06.517776  210951 pod_workers.go:190] "Error syncing pod, skipping" err="failed to \"StartContainer\" for \"metallb-speaker\" with CreateContainerConfigError: \"failed to sync secret cache: timed out waiting for the condition\"" pod="metallb/metallb-1627865247-speaker-2hxgj" podUID=93c48c5d-b468-4f4f-ac2d-c4ffab5ce0b2
start failed in pod metallb-1627865247-speaker-2hxgj_metallb(93c48c5d-b468-4f4f-ac2d-c4ffab5ce0b2): CreateContainerConfigError: failed to sync secret cache: timed out waiting for the condition

# kube-controller-manager
Aug 02 03:38:05 node001 kube-controller-manager[206408]: E0802 03:38:05.084665  206408 leaderelection.go:325] error retrieving resource lock kube-system/kube-controller-manager: Get "https://127.0.0.1:6443/apis/coordination.k8s.io/v1/namespaces/kube-system/leases/kube-controller-manager?timeout=5s": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)
GET https://apiserver:6443/api/v1/namespaces/metallb/secrets?fieldSelector=metadata.name%3Dmetallb-1627865247-memberlist&limit=500&resourceVersion=0 500 Internal Server Error in 13 milliseconds

# kube-apiserver
Aug 02 03:35:37 node001 kube-apiserver[206326]: E0802 03:35:37.796055  206326 cacher.go:419] cacher (*core.Secret): unexpected ListAndWatch error: failed to list *core.Secret: unable to transform key "/registry/secrets/kube-node-lease/default-token-fgkrw": invalid padding on input; reinitializing...



kiublet | failed to sync secret cache
kubelet | timed out waiting for the condition
kube-controller-manager | error retrieving resource lock kube-system/kube-controller-manager: Get "https://127.0.0.1:6443
kube-controller-mana | GET https://apiserver:6443 500 Internal Server Error in 13 milliseconds | Internal error occurred: invalid padding on input
apiserver | failed to list *core.Secret