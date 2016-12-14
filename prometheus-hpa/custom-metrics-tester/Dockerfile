FROM cargo.caicloud.io/caicloud/debian:jessie

# Set the default timezone to Shanghai
RUN echo "Asia/Shanghai" > /etc/timezone
RUN dpkg-reconfigure -f noninteractive tzdata

EXPOSE 9102
COPY custom-metrics-tester /
CMD ["./custom-metrics-tester"]
