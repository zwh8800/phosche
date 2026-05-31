FROM elasticsearch:8.17.0

RUN elasticsearch-plugin install --batch https://get.infini.cloud/elasticsearch/analysis-ik/8.17.0
