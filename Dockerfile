FROM centos:7

COPY tke-kms-plugin /

CMD ["/tke-kms-plugin"]
