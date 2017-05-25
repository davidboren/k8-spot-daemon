FROM scratch
ADD dist/k8-spot-daemon /bin/k8-spot-daemon
ENV PATH=/bin
ENV TMPDIR=/
CMD ["/bin/k8-spot-daemon", "run"]