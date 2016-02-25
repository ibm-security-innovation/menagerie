FROM java:7
RUN apt-get update
RUN apt-get install -y zip
COPY ./run.sh /engine/
RUN cd /engine; chmod +x run.sh;
RUN wget -q -O /usr/bin/apktool https://raw.githubusercontent.com/iBotPeaches/Apktool/master/scripts/linux/apktool
RUN wget -q -O /usr/bin/apktool.jar https://bitbucket.org/iBotPeaches/apktool/downloads/apktool_2.0.3.jar
RUN chmod +x /usr/bin/apktool*
VOLUME /var/data
WORKDIR /engine

