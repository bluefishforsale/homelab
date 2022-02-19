ansible -i inventory.ini etcd -a \
  'sudo etcdctl member list \
    --endpoints=https://127.0.0.1:2379  \
    --cacert=/var/lib/kubernetes/ca.pem \
    --cert=/var/lib/kubernetes/kubernetes.pem \
    --key=/var/lib/kubernetes/kubernetes-key.pem'
