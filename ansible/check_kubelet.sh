ansible -i inventory.ini k8s -a 'sudo kubectl get nodes --kubeconfig /var/lib/kubernetes/admin.kubeconfig'
