alias log="sudo --no-pager journalctl -u $@"
alias sys="sudo --no-pager systemctl $@"
alias tf=terraform
alias k=kubectl
alias ktx=kubectx
alias zshrc="source ${HOME}/.zshrc"
alias ls=lsd

export SYSTEMD_PAGER=
export PATH="/usr/local/bin:$PATH"