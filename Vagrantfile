Vagrant.configure(2) do |config|
  config.vm.box = "ubuntu/trusty64"

  config.vm.provider "virtualbox" do |v|
    v.memory = 2048
  end

  config.vm.network :forwarded_port, id: "frontend", guest: 80, host: 8100
  config.vm.network :forwarded_port, id: "ssh", guest: 22, host: 8222
  config.vm.network :forwarded_port, id: "dregistry", guest: 5000, host: 5000
  config.vm.network :forwarded_port, id: "rabbitmq", guest: 15672, host: 15672
  config.vm.provider :virtualbox do |vb|
    vb.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
  end
  config.vm.synced_folder ".", "/home/vagrant/src"

  config.vm.provision "shell", privileged: true, inline: <<EOF
    apt-get update -y
    apt-get install -y docker.io git
 
    sudo apt-key adv --keyserver hkp://pgp.mit.edu:80 --recv-keys 58118E89F3A912897C070ADBF76221572C52609D
    echo deb https://apt.dockerproject.org/repo ubuntu-trusty main > /etc/apt/sources.list.d/docker.list
    apt-get update
    apt-get install -y docker-engine
     
    echo 'DOCKER_OPTS="--userns-remap=default"' > /etc/default/docker
    cp /home/vagrant/src/deploy/sources/docker_sock.conf /etc/init/
    initctl reload-configuration
    service docker restart
    usermod -a -G docker vagrant
 
    curl -sSL https://github.com/docker/compose/releases/download/1.6.0/docker-compose-Linux-x86_64 > /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose
EOF

  config.vm.provision "shell", privileged: true, inline: <<EOF
    # install local registry, under root
    pushd /home/vagrant/src/registry
    bash -x install.sh
    sleep 3
    openssl s_client -connect localhost:5000 -showcerts </dev/null 2>/dev/null | openssl x509 -outform PEM # not sure why sometimes this call fails first time, so rather fail here before next steps
    bash -x ../deploy/reg-connect.sh --renew-certs --login-user=`head -1 /data/menagerie/volumes/registry/auth/cred` --login-pass=`tail -1 /data/menagerie/volumes/registry/auth/cred`
    su - vagrant -c 'echo "`id vagrant`"'
    su - vagrant -c '/home/vagrant/src/deploy/reg-connect.sh --login-user=`sudo head -1 /data/menagerie/volumes/registry/auth/cred` --login-pass=`sudo tail -1 /data/menagerie/volumes/registry/auth/cred`'
    popd
EOF

  config.vm.provision "shell", privileged: false, inline: <<EOF
    curl -sSL http://golang.org/dl/go1.5.1.linux-amd64.tar.gz | tar -xz -C $HOME

    echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
    echo 'export GOROOT=$HOME/go' >> ~/.profile
    mkdir $HOME/menagerie
    ln -s /home/vagrant/src /home/vagrant/menagerie/current
EOF

  config.vm.provision "shell", privileged: true, inline: <<EOF
    # build menagerie and demo engine containers. Note: we DO NOT run this under
    # privillaged:false, since this does executes the command under user vagrant,
    # but stripping the docker group. This causes docker-build to fail.
    su - vagrant -c 'cd /home/vagrant/src && ./build.sh'
    su - vagrant -c 'cd /home/vagrant/src/environments/generic-ami/demo.engine && ./build.sh'
    cp /home/vagrant/src/environments/generic-ami/demo.engine/menagerie.apktool /etc/apparmor.d/

    # finish install
    pushd /home/vagrant/src/deploy
    bash -x install.sh --restart-shared --reg-server=localhost:5000 --user=root --deploy-env=generic-ami
    popd
EOF

end
