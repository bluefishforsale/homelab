# how to run these playbooks

# example:
## single host
` ansible-playbook -i inventory.yaml -l "virtual.local," --ask-become-pass init.yaml