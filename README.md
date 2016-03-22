# Menagerie
An orchestration platform for Docker containers runnning batch jobs. This was driven
by a need to run multiple malware analyzers side by side, each with a different set
of installation requirements and technologies. These needed to be exposed as
services to other systems and users.

The following common services are added on top of the raw engines:
* Job submission and tracking
* Job history
* Execution and resource containment
* Engine version management


## Quick start

```
$ cd environments/generic-ami
$ vagrant up
```

* Launches the entire system, with a sample engine wrapping
  [apktool](http://ibotpeaches.github.io/Apktool/)
* We are using ubuntu 14.04 box and docker 1.10 - you can adapt it to your env
  but keep in mind at least docker 1.10 is expected, for features we are using
* It is recommended to install the
  [vagrant-vbguest](https://github.com/dotless-de/vagrant-vbguest) plugin - this
will align the guest additions in the imported box

Once the system is up and running it can be used as follows (from outside the
Vagrant box):
* Submit job to engine via `curl -v -XPOST http://localhost:8100/apktool/upload -F "upload=@<path-to-sample-apk>"`. The response is a `job-id` number
* Get response via `curl -v http://localhost:8100/result/<job-id>`
* Console and result viewer via [menagerie
  console](http://localhost:8100/console)
* RabbitMQ monitoring via [RabbitMQ admin](http://localhost:15672). Use the
  credentials provided in `confs/default.json`, by default `menagerie|menagerie`

----

## API
Menagerie supports the following HTTP calls:

| Method | URL                    | Parameters    | Result (JSON) |
| :----- |:----------------------:| :------------:|:-------------:|
| POST   | `/<engine-name>/upload` | `upload` (Multipart): `filename` and body|<ul><li>`jobid`: job tracking ID</li></ul>|
| GET    | `/result/<id>`         | `<id>` (URL): `jobid` from `upload` call |<ul><li>`status`: [`Running`,`Success`,`Failed`]</li><li>`summary`: excerpt from result file</li><li>`link`: link to result file</li></ul>|
| GET    | `/link/<id>`           | `<id>` (URL): `jobid` from `upload` call |Result file bytes|

`curl` calls:
* `curl -XPOST http://<server>:<port>/<engine-name>/upload -F "upload=@<path-to-file>"`
* `curl -XGET  http://<server>:<port>/result/<jobid>`
* `curl -XGET  http://<server>:<port>/link/<jobid>`

----

## Architecture
![alt text](../../raw/master/docs/images/arch.png "Architecture diagram")

The entire system is built with Docker containers that interact with each other.
The vagrant box loads all of them in one place, but some services can be
separated to external locations (see [scalability](#scalability) section below). Here is what
 we use, each running in its own container:
* Secure private Docker registry - for storing engines and the menagerie
  containers, as well as component version management
* RabbitMQ
* Mysql - for job tracking and history
* Go frontend - HTTP API, console webapp, and internal storage service
* Go backend worker threads that launch the engine containers

Job lifecycle is as follows:
* A file is submitted to specific engine over HTTP API
* The file is stored in a file storage over HTTP, a job is inserted into a named queue in
  RabbitMQ, and a job entry is created in Mysql
* Workers configured to monitor queues by name pull jobs when free. When a job is
  pulled:
  * A directory is created, and the input file is pulled from the storage
  * An engine container is launched, with the directory mounted as configured
  * When the engine is done - the worker acks RabbitMQ, stores the result,
    and sends a completion request over an internal HTTP API
  * The completion call updates the database as needed

---

## HOWTOS
### Adding engines
Creating additional engines is as easy as creating a Docker file. See the
sample wrapper we provide for apktools in
[here](environments/generic-ami/demo.engine).

Once the engine is built and pushed to the private registry, you need to:
* Add an entry in both 'engines.json' files (see below file system structure). An
  entry has the following structure (see also the [apktool engine
config](../../blob/master/environments/generic-ami/confs/engines.json)):

```
{"engines": [
  { "name": "apktool",                        # engine queue name
    "workers": 2,                             # how many workers listening
    "image": "{{regserver}}/apktool:stable",  # regserver:port/new-engine-name:tag
    "runflags": [                             # additional run flag strings, concatenated with spaces to cmd
      "--security-opt",
      "apparmor:menagerie.apktool"
    ],
    "cmd": "/engine/run.sh",                  # entry point
    "mountpoint": "/var/data",                # where the engine expects the input (is ephemeral docker volume)
    "sizelimit": 50000000,                    # limit on upload size, bytes
    "inputfilename": "sample.apk",            # the script should expect a single file as input, by this name
    "user": {{uid}},                          # templated, using the UID provided to install. can be HC
    "timeout": 240                            # seconds on engine run
  }
]}
```

* Pull the engine container in the deployed machine
* `sudo stop menagerie && sudo start menagerie`

You will see a new engine/queue added to the [console](http://localhost:8100/console)
and [RabbitMQ admin](http://localhost:15672). Submitting jobs is the same as
described in the [quick start](#quick-start), use the `engine-name` instead of
`apktool` in the `curl` request.

When you build and push new versions of engine containers, follow the steps
above, simply correcting the config to point to the image tag you want (note if the
config is using tag `latest` - merely pulling the latest image is enough no need
to restart services).


### Configuration
The menagerie container is launched twice as part of normal operation - once as
the frontend server and once as the workers controller. For each of the
two instances we push the same two config files into the container, located under
`/data/confs` (see also [volumes section](volumes)).

For engine configuration - see the [engines](#adding-engines) section above.

The second file is global configuration file
([default.json](../../blob/master/environments/generic-ami/confs/default.json)
that contains location of intenal services, credentials, and misc directories.

Cleanup script (for volumes) is located inside the menagerie containers under `/usr/local/menagerie/scripts/cleanup.sh` -
edit this file if you need to increase/reduce the period files are kept. This script is launched periodically via an
external cron task see `/etc/cron.d/menagerie-cron`

### Deployment
It is recommended to read the
[Vagrantfile](../../blob/master/environments/generic-ami/Vagrantfile) as this
contains the most updated example of deploying the system. Note that vagrant
places all required services in one box - this can be distributed as described
[below](#scalability). In addition note that the vagrant box also functions as
a dev/build box as we compile the Go binaries on it. This is not required in
a production deployments where the menagerie container can be built on
a separate machine and pushed to the Docker registry (we use Jenkins, feel free
to use your fancy).

#### Volumes
We are using docker named volumes that are mounted under `/data`
inside the core containers:
```
vagrant@vagrant-ubuntu-trusty-64:~$ docker volume ls
DRIVER              VOLUME NAME
local               menagerie_menage
local               menagerie_mysql
local               menagerie_rabbitmq
local               mngreg_registry_conf
local               mngreg_registry_data
local               menagerie_frontend
```

Structure inside the frontend volume:
```
frontend_1$ tree /data/
.
└── data/
    ├── keys/
    │   ├── engines.json
    │   └── frontend.json
    ├── log/
    ├── mule/
    └── store/
       ├── <<job-id>>/
       │   ├── input
       │   └── result
       └── ...
```

Structure inside the backend/menage volume:
```
menage_1$ tree /data/
.
└── data/
    ├── keys/
    │   ├── engines.json
    │   └── frontend.json
    ├── log/
    ├── mule/
    └── jobs/
       ├── <<running/failed job-id>>/
       └── ...
```

Docker volumes are supported from version 1.9, and allows to persist data in Dockerland. These volumes can also be
remoted later on when using swarm or other scaling solutions.

#### Services
The containers are launched by Docker compose, monitored by upstart. The upstart
scripts are located under `/etc/init/` and are all named `menagerie*.conf`

#### Logs
all logs can be viewed using the `docker logs <container-name>` command. The maintainance logs that are launched via cron
are tagged with container name and can be viewed in `/var/log/syslog`

---

## Extra info
### Security
If we break aside the Mysql/RabbitMQ/Docker-registry containers (all can run as external
services in production environments, see [scalability](#scalability)), the
2 core services running are `frontend` and `menage`, the worker controller.

We ran the [docker-benchmark](https://github.com/docker/docker-bench-security)
test and remediated relevant comments (not all are relevant on
a developer/vagrant box). Note that it is highly recommended to run
this on your production deployment.

We running docker with the user namespace mapping (`DOCKER_OPTS="--userns-remap=default"`, see `Vagrantfile`), 
a new feature in 1.10. This means that although internally the containers are running as root user, the UID is actually
that of a less privillaged user on the host. The engines are also confined with timeout, and we highly recomment to add additional 
run flags to limit them via the JSON file described above.

Since we are using Docker volumes, there is no mapping of host disk into the containers.

Finally, we implemented apparmor profiles for the `menage`/`frontend` containers, and provide a [hands on wiki](../../wikis/engine_apparmor_setup) for adding apparmor profile
to your custom engines. Profiles are defined in `complain` mode.

### Scalability
![alt text](../../raw/master/docs/images/distribute.png "Distributed architecture diagram")

As mentioned earlier, in real production environments we should break out the
generic components:
* Mysql
* RabbitMQ
* Docker registry
* File store

Once we have that, we can have multiple nodes running only menagerie and engine
containers, and place a load-balancer in front of the API.

Note that when using a private Docker registry - you need to make sure the
certificate is installed and a `docker login` was performed. This can be done via the 
script `reg-connect.sh`, which is also used in the Vagrantfile.

---

## Contribution
Contributions to the project are welcomed. It is required however to provide alongside the pull request one of the contribution forms (CLA) that are a part of the project.
If the contributor is operating in his individual or personal capacity, then he/she is to use the [individual CLA](./CLA-Individual.txt);
if operating in his/her role at a company or entity, then he/she must use the [corporate CLA](CLA-Corporate.txt).

---

## License

(c) Copyright IBM Corp. 2015, 2016

This project is licensed under the Apache License 2.0.  See the
[LICENSE](LICENSE) file for more info.

3rd party software used by menagerie:

* [Docker](https://github.com/docker/docker/blob/master/LICENSE): Apache 2.0 license
* [The Go Programming Language](https://golang.org): BSD-style license
* [go-sql-driver](https://github.com/go-sql-driver/mysql):  MPL 2.0 icense
* [glog](https://github.com/google/glog): Apache 2.0 icense
* [amqp](https://github.com/streadway/amqp): BSD-style license
* [bootstrap](http://getbootstrap.com/getting-started/#license-faqs): MIT license
* [bootstrap-material-design](https://github.com/FezVrasta/bootstrap-material-design): MIT license
* [jsrender](https://github.com/BorisMoore/jsrender/blob/master/MIT-LICENSE.txt): MIT license
* [twbs-pagination](https://github.com/esimakin/twbs-pagination/blob/develop/LICENSE): Apache 2.0 license
* [looplab/fsm](https://github.com/looplab/fsm): Apache 2.0 license
* [zenazn/goji](https://github.com/zenazn/goji): MIT license

Project icon from [iconka](http://iconka.com/en/downloads/cat-power/), under [free license](http://iconka.com/en/licensing/)
