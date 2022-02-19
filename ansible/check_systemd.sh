ansible -i inventory.ini k8s -a 'sudo systemctl status haproxy keepalived etcd kubelet kube-proxy kube-apiserver kube-controller-manager containerd' | egrep  '(●|Active)'
