import json
import sys


def request(operation):
    print(json.dumps({"$rcp": "db", **operation}), flush=True)
    reply = json.loads(sys.stdin.readline())
    if not reply["ok"]:
        raise RuntimeError(reply["error"])
    return reply


event = json.loads(sys.stdin.readline())
request({"op": "put", "collection": "visits", "key": "count", "value": 1})
item = request({"op": "get", "collection": "visits", "key": "count"})["item"]
print(json.dumps({
    "statusCode": 200,
    "headers": {"content-type": "application/json"},
    "body": json.dumps({"count": item["value"]}),
}), flush=True)
