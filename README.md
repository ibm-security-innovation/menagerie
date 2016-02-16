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
* We used CentOS 6.6 to be as close as possible to an AMI environment - you can
  adapt it to your env
* It is recommended to install the
  [vagrant-vbguest](https://github.com/dotless-de/vagrant-vbguest) plugin - this
will align the guest additions in the imported box

Once the system is up and running it can be used as follows (from outside the
Vagrant box):
* Submit job to queue via `curl -v -XPUT http://localhost:8100/apktool/upload --data-binary @<path-to-sample-apk>`. The response is a `job-id` number
* Get response via `curl -v http://localhost:8100/result/<job-id>`
* Console and result viewer via [menagerie
  console](http://localhost:8100/console)
* RabbitMQ monitoring via [RabbitMQ admin](http://localhost:15672). Use the
  credentials provided in `confs/default.json`, by default `menagerie|menagerie`

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
  - name:           # engine queue name
    workers:        # how many workers listening 
    image:          # regserver:port/new-engine-name:tag
    cmd:            # engine activation command inside the container
    mountpoint:     # where the engine expects the input
    sizelimit:      # limit on upload size, bytes
    inputfilename:  # the script should expect a single file as input, by this name
    user:           # UID to execute
    runflags:       # additional run flag string, embedded as-is into 'docker run' cmd
    timeout:        # on job execution
```

* Pull the engine container in the deployed machine
* `sudo stop menagerie && sudo start menagerie`

You will see a new queue added to the [console](http://localhost:8100/console)
and [RabbitMQ admin](http://localhost:15672). Submitting jobs is the same as
described in the [quick start](#quick-start), use the `queue-name` instead of
`apktool` in the `curl` request.

When you build and push new versions of engine containers, follow the steps
above, simply correcting the config to point to the image tag you want (note if the
config is using tag `latest` - merely pulling the latest image is enough no need
to restart services).


### Configuration
The menagerie container is launched twice as part of normal operation - once as
the frontend server and once as the workers controller. For each of the
two instances we have the same two config files, located under
`/data/menagerie/volumes/[menage/frontend]/confs` (see also [volumes
section](volumes)).

For engine configuration - see the [engines](#adding-engines) section above.

The second file is global configuration file
([default.json](../../blob/master/environments/generic-ami/confs/default.json)
that contains location of intenal services, credentials, and misc directories.

Cleanup configuration (for file system) is located under `/data/menagerie/scripts/cleanup.sh` -
edit this file if you need to increase/reduce cleanup times. This script is launched every minute.

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
In the heart of the system we have volumes that are mounted under `/var/data`
inside the core containers:
```
$tree /data/menagerie/
.
├── scripts/
└── volumes/
    ├── frontend/
    │   ├── keys/
    │   │   ├── engines.json
    │   │   └── frontend.json
    │   ├── log/
    │   ├── mule/
    │   └── store/
    │       ├── <<job-id>>/
    │       │   ├── input
    │       │   └── result
    │       └── ...
    ├── menage/
    │   ├── jobs/
    │   ├── keys/
    │   │   ├── engines.json
    │   │   └── menage.json
    │   ├── log/
    │   ├── mule/
    │   ├── menagerie-compose.yml
    │   └── log.sh
    ├── mysql/...
    ├── rabbitmq/...
    └── registry/...
```


#### Services
The containers are launched by Docker compose, monitored by upstart. The upstart
scripts are located under `/etc/init/` and are all named `menagerie*.conf`

#### Logs
See above log directories. The utility `.../volumes/menage/log.sh` combines
multiple log files into a linear time sorted view.

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

The `frontend` services is running as a container under a user account UID
(configurable see `install.sh`, we use `vagrant` in the vagrant installation).
This is the only service that is exposed to the outside, and so in production
the actual user should be with minimal privillages. We also recommend to place
an nginx server in front of the API, and enfoce client side certificates to
limit access even more.

The `menage` service is running as a container under the root UID. Reason for
not using a lesser privilage is the need for this container to access the Docker
socket and launch additional (engine) containers. The only extrenal input comes
from the message queue, so it has a minimal surface area for exploits.

The engines themselves run under a limited user UID (configurable, `vagrant` in
the demo system). Each engine has a very limited view of the file system (we only mount
`.../volumes/menage/jobs/job_id`) and is limited in execution time. After each
run the container is destroyed, keeping only the result file. 

We are now working on porting to Docker 1.10. This will enable us to use
the new user-namespaces feature thus reducing concerns on running as root inside
containers. There are several other security improvements that will come with
this release.

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

Project icon from [iconka](http://iconka.com/en/downloads/cat-power/), under [free license](http://iconka.com/en/licensing/)
