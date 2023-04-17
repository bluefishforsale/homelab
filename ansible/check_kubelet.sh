ansible -i inventory.ini k8s_controller -a 'sudo kubectl get nodes --kubeconfig /var/lib/kubernetes/admin.kubeconfig'
