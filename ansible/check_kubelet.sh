ansible -i inventory.yaml k8s -a 'sudo kubectl get nodes --kubeconfig /var/lib/kubernetes/admin.kubeconfig'
