ansible -i inventory.yaml k8s -a 'sudo systemctl status haproxy keepalived kubelet kube-proxy kube-apiserver kube-controller-manager containerd' | egrep  '(●|Active)'
