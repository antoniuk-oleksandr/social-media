"""AWS deployment diagram for the Theads-like application.

Requires:
    pip install diagrams
    Graphviz installed and available as the dot executable.

Run:
    python deployment_diagrams.py
"""

from diagrams import Cluster, Diagram, Edge
from diagrams.aws.compute import Lambda
from diagrams.aws.database import Dynamodb, ElasticacheForRedis
from diagrams.aws.devtools import XRay
from diagrams.aws.integration import SimpleQueueServiceSqs
from diagrams.aws.management import Cloudwatch
from diagrams.aws.network import APIGateway
from diagrams.aws.storage import SimpleStorageServiceS3
from diagrams.generic.compute import Rack


with Diagram(
    "Theads-like AWS Deployment",
    filename="deployment_aws",
    direction="TB",
    show=False,
    graph_attr={
        "splines": "ortho",
        "nodesep": "0.8",
        "ranksep": "1.0",
        "pad": "0.4",
    },
):
    client = Rack("Web or mobile client")

    with Cluster("AWS account and region"):
        gateway = APIGateway("API Gateway")

        with Cluster("MVP Lambda functions"):
            auth = Lambda("Auth")
            users = Lambda("Users")
            posts = Lambda("Posts")
            comments = Lambda("Comments")
            ads = Lambda("Ads")
            feed = Lambda("Feed")

        dynamodb = Dynamodb("DynamoDB")

        with Cluster("Phase 2 observability"):
            cloudwatch = Cloudwatch("CloudWatch")
            adot = Lambda("ADOT extension")
            xray = XRay("X-Ray")

        with Cluster("Deferred services"):
            s3 = SimpleStorageServiceS3("S3 media")
            sqs = SimpleQueueServiceSqs("SQS processing")
            redis = ElasticacheForRedis("Redis cache")

    grafana = Rack("Grafana")

    client >> Edge(label="HTTPS") >> gateway
    gateway >> [auth, users, posts, comments, ads, feed]

    auth >> dynamodb
    users >> dynamodb
    posts >> dynamodb
    comments >> dynamodb
    ads >> Edge(label="admin only") >> dynamodb
    feed >> Edge(label="posts and ads") >> dynamodb

    # These representative edges apply to all Lambda functions in the cluster.
    feed - Edge(label="Zap JSON logs from all Lambdas", style="dashed") - cloudwatch
    feed - Edge(label="OpenTelemetry from all Lambdas", style="dashed") - adot
    adot >> xray
    cloudwatch >> grafana

    posts - Edge(label="Phase 3 attachments", style="dashed") - s3
    comments - Edge(label="Phase 3 attachments", style="dashed") - s3
    users - Edge(label="Phase 3 avatars", style="dashed") - s3
    s3 - Edge(style="dashed") - sqs
    feed - Edge(label="Phase 4", style="dashed") - redis
    posts - Edge(label="Phase 4", style="dashed") - redis
