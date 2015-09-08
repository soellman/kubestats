FROM busybox
MAINTAINER Oliver Soell <oliver@soell.net>

ADD kubestats /kubestats
ENTRYPOINT [ "/kubestats" ]
