**Taken from http://dmitrypol.github.io/redis/2020/10/01/redis-sentinel.html**

I am a Sr. Software Developer at Oracle Cloud. The opinions expressed here are my own and not necessarily those of my employer.

# Monitoring Redis with Sentinels

*Oct 1, 2020*

Running Redis in production can be a complicated undertaking. Ideally our cloud provider will offer a managed service but sometimes it is not an option. In this article we will expore how to run Redis and monitor it ourselves.

Redis Sentinel provides high availability for Redis. This allows us to setup Redis instances that can recover from certain types of failures. The advantage of this approach is that we can control the exact version of Redis we will be running. We also will be able to connect to our Redis instances and do extensive troubleshooting if necessary.

<ul>
<li>Local environment</li>
<li>Sentinel dockerfile</li>
<li>Setting up Redis replication</li>
<li>Configuring Redis for Sentinel monitoring</li>
<li>Testing the failover process</li>
<li>sentinel.conf</li>
<li>Running Redis in production</li>
    <ul>
      <li>Adding instances</li>
      <li>Removing instances</li>
      <li>Increasing / decreasing amount of RAM on each instance</li>
      <li>Upgrading / downgrading Redis version</li>
    </ul>
<li>Links</li>
</ul>

## Local environment

We will prototype it in local environment with docker-compose. Save this as docker-compose.yml.

```yml
version: '3.7'
services:
  redis1:
    container_name: redis1
    image: redis:6.0.8-alpine
  redis2:
    container_name: redis2
    image: redis:6.0.8-alpine
  redis3:
    container_name: redis3
    image: redis:6.0.8-alpine
  sentinel1:
    container_name: sentinel1
    build:
      context: .
      dockerfile: sentinel.Dockerfile
  sentinel2:
    container_name: sentinel2
    build:
      context: .
      dockerfile: sentinel.Dockerfile
  sentinel3:
    container_name: sentinel3
    build:
      context: .
      dockerfile: sentinel.Dockerfile
```

## Sentinel dockerfile

Redis Sentinel requires a local sentinel.conf file. Save this as sentinel.Dockerfile and run `docker-compose up --build -d`.

```dockerfile
FROM redis:6.0.8-alpine
RUN touch /etc/sentinel.conf
RUN chmod a+w /etc/sentinel.conf
RUN echo -e '#!/bin/sh \nset -e \n/usr/local/bin/redis-sentinel /etc/sentinel.conf \nexec "$@"' > /usr/local/bin/docker-entrypoint.sh
EXPOSE 26379
```

## Setting up Redis replication

Now we can manually setup replication between Redis instances.

```bash
# make redis2 replica of redis1
docker exec -it redis2 sh
redis-cli
replicaof redis1 6379
# repeat for redis3
docker exec -it redis3 sh
redis-cli
replicaof redis1 6379
# check replicaton on redis1
docker exec -it redis1 sh
redis-cli
info replication
# should say
role:master
connected_slaves:2
...
```

## Configuring Redis for Sentinel monitoring

Now we need to register Redis primary (master) with each Redis Sentinel. Sentinel will then automatically discover the replicas.

```bash
docker exec -it sentinel1 sh
ping redis1
# make note of the IP address
redis-cli -p 26379
sentinel masters
# returns
(empty array)
sentinel monitor my_redis IP_ADDRESS_HERE 6379 2
sentinel masters
# responds with:
1)  1) "name"
    2) "my_redis"
    3) "ip"
    4) "IP_ADDRESS_HERE"
...
# check that sentinel is aware of replicas, this will respond with 2 records
sentinel replicas my_redis
# repeat the process on sentinel2 and sentinel3
docker exec -it sentinel2 sh
...
docker exec -it sentinel3 sh
...
# check that other 2 sentinels are also aware of these Redis instances
sentinel sentinels my_redis
```

## Testing the failover process

We can subscribe to Redis Sentinels events via Pub/Sub in one bash tab:

```bash
docker exec -it sentinel1 sh
redis-cli -p 26379
psubscribe *
Reading messages... (press Ctrl-C to quit)
1) "subscribe"
2) "*"
3) (integer) 1
```

We can also subscribe to other Sentinels by repeating the step above in separate bash tabs. Now we will simulate Redis failure on our primary instance in another bash tab.

```bash
docker exec -it redis1 sh
redis-cli
debug sleep 60
In the first bash tab where we subscribe to Redis Sentinel we will see:

1) "pmessage"
2) "*"
3) "+sdown"
4) "master my_redis 192.168.32.7 6379"
...
1) "pmessage"
2) "*"
3) "+vote-for-leader"
4) "42b03403466dd9e2d929e779a7f62d7584e5bc22 1"
1) "pmessage"
2) "*"
3) "+odown"
4) "master my_redis 192.168.32.7 6379 #quorum 3/2"
1) "pmessage"
2) "*"
3) "-role-change"
4) "slave 192.168.32.5:6379 192.168.32.5 6379 @ my_redis 192.168.32.7 6379 new reported role is master"
...
1) "pmessage"
2) "*"
3) "+switch-master"
4) "my_redis 192.168.32.7 6379 192.168.32.5 6379"
...
```

We can see how Sentinels perform various tasks in the process. Separately we could write another tool to capture these messages and alert our engineers appropriately but that is outside the scope of this article.

Now if we switch back to redis1 to check replication status we will see it has been demoted to replica.

```bash
info replication
# Replication
role:slave
master_host:IP_HERE
master_port:6379
...
```

We can connect to redis2 and redis3 and one of them should be the new primary. We can use debug sleep 60 command to perform another failover.

We can also ask Sentinels who is the current primary via sentinel get-master-addr-by-name my_redis. We can force failover by sending sentinel failover my_redis command to a specific Sentinel. If we check get-master-addr-by-name we will get new information. Other Sentinels will also be informed of the new primary. Another useful command is sentinel ckquorum my_redis which should return OK 3 usable Sentinels. Quorum and failover authorization can be reached.

## sentinel.conf

Sentinel will write updated info to sentinel.conf file overwriting the blank file we created during docker build.

```bash
docker exec -it sentinel1 sh
cat /etc/sentinel.conf
port 26379
user default on nopass ~* +@all
dir "/data"
sentinel myid 655baee6388f2825b99eba4d309cac81d75b0f89
sentinel deny-scripts-reconfig yes
sentinel monitor my_redis 192.168.32.5 6379 2
...
```

## Running Redis in production

Now that we prototyped this locally we need to setup appropriate prod infrastructure. Sentinels do not require that much capacity so we can make those instances smaller. But we will need to carefully think how much data we will likely store in our Redis instances and get enough RAM.

## Adding instances

In production we will need to launch new instances. We can practice this locally by modifying our docker-compose.yml and running docker-compose up --build -d.

```yml
...
  redis4:
    container_name: redis4
    image: redis:6.0.8-alpine
...
```

Now we need to find out the current primary by running this against various Redis containers docker exec -it redis2 redis-cli info replication and checking for response containing role:master.

Then we can make new container a replica of current primary docker exec -it redis4 redis-cli replicaof REDIS_PRIMARY_NAME_HERE 6379.

If we run docker exec -it REDIS_PRIMARY_NAME_HERE redis-cli info replication we will see connected_slaves:3

We can check that Sentinels automatically became aware of the replica with docker exec -it sentinel1 redis-cli -p 26379 sentinel replicas my_redis. The output should contain 3 records.

## Removing instances

In production we will terminate Redis instances but locally we will run docker stop redis4. Now docker exec -it REDIS_PRIMARY_NAME_HERE redis-cli info replication we tell us connected_slaves:2.

But docker exec -it sentinel1 redis-cli -p 26379 sentinel replicas my_redis still has 3 records. One of them should contain 9) "flags" 10) "s_down,slave". That is the container we just killed but Sentinel still knows about it. Which is what we want if this was unintentional. If we subscribed to Sentinel pubsub we would have received a message:

```bash
1) "pmessage"
2) "*"
3) "+sdown"
4) "slave 172.19.0.8:6379 172.19.0.8 6379 @ my_redis 172.19.0.5 6379"
```

To make Sentinel forget about this instance that we killed manually we need to docker exec -it sentinel1 redis-cli -p 26379 sentinel reset my_redis. Repeat the process for sentinel2 and sentinel3. Now running docker exec -it sentinel1 redis-cli -p 26379 sentinel replicas my_redis will return only 2 records.

## Increasing / decreasing amount of RAM on each instance

* If decreasing the amount of RAM we need to make sure that the amount of data currently stored in Redis can fit into the new amount of RAM.

* The process is largely a repetition of various steps above.

* Launch new instances with appropriate amount of RAM.

* Make new instances replicas of the current primary.

* Terminate original replicas one at a time.

* Stop if there are any issues and rollback.

* Failover the primary by sending sentinel failover my_redis to one of the Sentinels and terminate the instance.

* Do sentinel reset my_redis on all Sentinels.

## Upgrading / downgrading Redis version

* The process is also largely a repetition of various steps above.

* Launch new instances with new version of Redis.

* Make new instances replicas of the current primary.

* Terminate original replicas one at a time.

* Stop if there are any issues and rollback.

* Failover the primary (with previous Redis version) and terminate the instance.

* Do sentinel reset my_redis on all Sentinels.

## Links

* https://redis.io/topics/sentinel

* https://github.com/redis/redis/blob/unstable/sentinel.conf