FROM ubuntu:14.04
RUN apt-get update && apt-get install -y wget dpkg libapparmor1 iptables
RUN wget -O docker.deb https://apt.dockerproject.org/repo/pool/main/d/docker-engine/docker-engine_1.7.1-0~trusty_amd64.deb 
RUN dpkg -i docker.deb 
COPY ./bin /usr/local/menagerie/bin
COPY ./console /usr/local/menagerie/console
WORKDIR /usr/local/menagerie
