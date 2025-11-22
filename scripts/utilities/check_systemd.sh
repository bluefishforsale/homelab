# ansible -i inventory.ini k8s -a 'sudo systemctl list-units --all  2>&1 | egrep "(haproxy|keepalived|etcd|kubelet|kube-proxy|kube-apiserver|kube-controller-manager|containerd)"'
# ansible -i inventory.ini k8s -a 'sudo systemctl status haproxy keepalived etcd kubelet kube-proxy kube-apiserver kube-controller-manager containerd' | egrep '(●|Active:|Loaded:)'
ansible -i inventory.ini k8s -a 'sudo systemctl status haproxy keepalived etcd kubelet kube-proxy kube-apiserver kube-controller-manager containerd'
