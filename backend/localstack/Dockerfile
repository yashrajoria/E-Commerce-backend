# Pin to the last pre-transition release so local development does not require
# a LocalStack auth token.
FROM localstack/localstack:4.14.0

# Use COPY instead of inline RUN printf to avoid the maintenance burden of
# escaping every line, and to avoid accidentally regressing the script.
# CRLF issues on Windows hosts are handled via .gitattributes (see below).
#
# Add this to your .gitattributes to ensure LF line endings on all hosts:
#   localstack/init/ready.d/10-bootstrap-resources.sh text eol=lf

RUN mkdir -p /etc/localstack/init/ready.d

COPY init/ready.d/10-bootstrap-resources.sh /etc/localstack/init/ready.d/10-bootstrap-resources.sh
RUN chmod +x /etc/localstack/init/ready.d/10-bootstrap-resources.sh