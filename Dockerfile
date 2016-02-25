FROM ubuntu:14.04
RUN apt-get update && apt-get install -y wget dpkg libapparmor1 iptables libsystemd-journal0
RUN wget -q -O docker.deb https://apt.dockerproject.org/repo/pool/main/d/docker-engine/docker-engine_1.10.0-0~trusty_amd64.deb 
RUN dpkg -i docker.deb 
RUN ln -s /data/keys/.docker /root/.docker 
RUN mkdir /usr/local/menagerie
WORKDIR /usr/local/menagerie
COPY ./console /usr/local/menagerie/console
COPY ./bin /usr/local/menagerie/bin
COPY ./scripts /usr/local/menagerie/scripts
