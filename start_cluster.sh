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

make neb-docker-build-plugin
make neb-docker-push-plugin

kubectl label node ub-10 nos.nebuly.com/gpu-partitioning=mps
helm install deployments/helm/nvidia-device-plugin --version 0.13.0 --generate-name -n nebuly-nvidia --create-namespace

# kubectl label node ub-10 mps-gpu-enabled=true   # Add device plugin label

# kubectl create -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v0.13.0/nvidia-device-plugin.yml
# kubectl create sa mps-device-plugin-manager -n kube-system
# kubectl create clusterrolebinding mps-device-plugin-manager-role --clusterrole=cluster-admin --serviceaccount=kube-system:mps-device-plugin-manager
# kubectl apply -f mps-manager.yaml