FROM ubuntu:14.04
MAINTAINER "Joshua C. Randall" <jcrandall@alum.mit.edu>

# Install prerequisite apt packages
RUN \
  apt-get -q=2 update && \
  apt-get -q=2 upgrade && \
  apt-get -q=2 install -y --no-install-recommends \
    ca-certificates \
    git \
    golang-go

# Create and set GOPATH
RUN \
  mkdir /var/go
ENV GOPATH /var/go
ENV GOBIN /usr/local/bin

# Install prerequisite go packages
RUN \
  go get \
    github.com/dmotylev/goproperties \
    github.com/gorilla/mux \
    github.com/jmoiron/sqlx \
    github.com/ziutek/mymysql/godrv

# Build and install hgi-vrtrack-qc-server
ADD . /opt/hgi-vrtrack-qc-server
WORKDIR /opt/hgi-vrtrack-qc-server
RUN \
  go install hgi-vrtrack-qc-server.go

# Set default command
CMD ["hgi-vrtrack-qc-server"]

