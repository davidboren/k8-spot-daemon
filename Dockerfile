FROM centurylink/ca-certs
ADD dist/k8-spot-daemon-linux-x86 /bin/k8-spot-daemon-linux-x86
ENV PATH=/bin
ENV TMPDIR=/
CMD ["/bin/k8-spot-daemon-linux-x86", "run"]