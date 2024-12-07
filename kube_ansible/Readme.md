
# Validate
ansible-playbook -i inventory.ini  --syntax-check 01_kube_apt_repo.yaml
ansible-playbook -i inventory.ini  --syntax-check 02_install_kubernetes.yaml
ansible-playbook -i inventory.ini  --syntax-check 03_containerd_and_networking.yaml
ansible-playbook -i inventory.ini  --syntax-check 04_configure_ha_proxy_keepalived.yaml
ansible-playbook -i inventory.ini  --syntax-check 05_initialize_master.yaml
ansible-playbook -i inventory.ini  --syntax-check 06_join_other_nodes.yaml
ansible-playbook -i inventory.ini  --syntax-check 07_configure_gpu_node.yaml

# Dry-Run
ansible-playbook -i inventory.ini 01_kube_apt_repo.yaml  --check
ansible-playbook -i inventory.ini 02_install_kubernetes.yaml  --check
ansible-playbook -i inventory.ini 03_containerd_and_networking.yaml  --check
ansible-playbook -i inventory.ini 04_configure_ha_proxy_keepalived.yaml  --check
ansible-playbook -i inventory.ini 05_initialize_master.yaml  --check

# Apply
ansible-playbook -i inventory.ini 01_kube_apt_repo.yaml
ansible-playbook -i inventory.ini 02_install_kubernetes.yaml
ansible-playbook -i inventory.ini 03_containerd_and_networking.yaml
ansible-playbook -i inventory.ini 04_configure_ha_proxy_keepalived.yaml
ansible-playbook -i inventory.ini 05_initialize_master.yaml
ansible-playbook -i inventory.ini 06_join_other_nodes.yaml
ansible-playbook -i inventory.ini 07_configure_gpu_node.yaml

 
# RESET
ansible -i inventory.ini k8s  -b -a 'sudo pgrep kube* | xargs sudo kill -9'
ansible -i inventory.ini k8s  -b -a 'sudo crictl stopp $(sudo crictl ps -a -q)'
ansible -i inventory.ini k8s  -b -a 'sudo crictl rmp $(sudo crictl ps -a -q)'
ansible -i inventory.ini k8s  -b -a 'sudo systemctl stop kubelet containerd'
ansible -i inventory.ini k8s  -b -a 'sudo ip link delete flannel.1'
ansible -i inventory.ini k8s  -b -a 'sudo rm -rf /etc/cni/net.d /var/lib/cni /var/lib/etcd /var/lib/kubelet /etc/kubernetes /var/lib/containerd'
ansible -i inventory.ini k8s  -b -a 'sudo kubeadm reset --force'
ansible -i inventory.ini k8s  -b -a 'sudo ipvsadm --clear'

# watching things
watch sudo crictl ps -a
watch sudo crictl logs -f containerid
sudo journalctl -fu kubelet

watch  sudo curl -s --cacert /etc/kubernetes/pki/ca.crt --cert /etc/kubernetes/pki/apiserver-kubelet-client.crt --key /etc/kubernetes/pki/apiserver-kubelet-client.key https://192.168.1.99:6443/healthz

watch curl -s http://127.0.0.1:2381/health

sudo ss -plant | grep 6443 | grep LIST