
# node migration insttructions
# https://docs.ondat.io/docs/operations/etcd/migrate-etcd-cluster/


export ETCDCTL_API=3
export endpoints=https://127.0.0.1:2379
export ETCDCTL_CACERT=/etc/etcd/ca.pem
export ETCDCTL_CERT=/etc/etcd/kubernetes.pem
export ETCDCTL_KEY=/etc/etcd/kubernetes-key.pem

exit

ETCDCTL_API=3 etcdctl member list --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/etcd/ca.pem \
  --cert=/etc/etcd/kubernetes.pem \
  --key=/etc/etcd/kubernetes-key.pem



ETCDCTL_API=3 etcdctl del /registry/secrets/kube-node-lease/default-token-fgkrw \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/etcd/ca.pem \
  --cert=/etc/etcd/kubernetes.pem \
  --key=/etc/etcd/kubernetes-key.pem



etcdctl get --prefix --keys-only /
etcdctl member list --endpoints=https://127.0.0.1:2379
etcdctl snapshot save ~/etcd.backup


# delete all etcd
ETCDCTL_API=3 \
  etcdctl \
    --cacert=/etc/etcd/ca.pem \
    --cert=/etc/etcd/kubernetes.pem \
    --key=/etc/etcd/kubernetes-key.pem \
    --endpoints=https://127.0.0.1:2379 \
    get --prefix --keys-only /  |\
  xargs -n1 -I{} -P9999 etcdctl \
    --cacert=/etc/etcd/ca.pem \
    --cert=/etc/etcd/kubernetes.pem \
    --key=/etc/etcd/kubernetes-key.pem \
    --endpoints=https://127.0.0.1:2379  del {}