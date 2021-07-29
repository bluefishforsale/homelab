  kubectl config set-cluster k8s \
    --certificate-authority=files/cfssl/ca.pem \
    --embed-certs=true \
    --server=https://apiserver:6443

  kubectl config set-credentials admin \
    --client-certificate=files/cfssl/admin.pem \
    --client-key=files/cfssl/admin-key.pem

  kubectl config set-context k8s \
    --cluster=k8s \
    --user=admin

  kubectl config use-context k8s