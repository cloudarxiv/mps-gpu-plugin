#!/bin/bash

# Setup kubernetes cluster
sudo kubeadm reset   # Delete existing master
rm $HOME/.kube/config
sudo rm -rf /etc/cni/net.d
sudo swapoff -a     # Swapoff
sudo kubeadm init --pod-network-cidr=10.244.0.0/16  # Initialize cluster
mkdir -p $HOME/.kube    
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
kubectl apply -f https://raw.githubusercontent.com/flannel-io/flannel/master/Documentation/kube-flannel.yml     # Use flannel for networking
kubectl taint nodes ub-10 node-role.kubernetes.io/master-   # Allow device plugins and pods to run on master

kubectl label node ub-10 nos.nebuly.com/gpu-partitioning=mps
helm install https://cloudarxiv.github.io/mps-gpu-plugin/nvidia-device-plugin-0.13.0.tgz \
  --version 0.13.0 \
  --generate-name \
  -n nebuly-nvidia \
  --create-namespace