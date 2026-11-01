FROM httpd:2.4.69
COPY apache.conf conf/extra/sensu.conf
COPY sleep broken cgi-bin/
RUN echo 'Include conf/extra/sensu.conf' >> conf/httpd.conf && chmod 755 cgi-bin/sleep cgi-bin/broken
COPY check-apache-status /usr/local/bin/
