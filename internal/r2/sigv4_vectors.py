# Reference SigV4 values from botocore, for the Go r2 client tests.
import datetime, hashlib
from unittest import mock
from botocore.auth import S3SigV4Auth, S3SigV4QueryAuth
from botocore.awsrequest import AWSRequest
from botocore.credentials import Credentials

creds = Credentials("AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY")
fixed = datetime.datetime(2026, 11, 15, 8, 30, 0)

with mock.patch("botocore.auth.get_current_datetime", return_value=fixed):
    pass
    pass
    body = b"<!DOCTYPE html><p>ol\xc3\xa1</p>"
    req = AWSRequest(method="PUT", url="https://acct123.r2.cloudflarestorage.com/faz-sites/sites/abc/v1/index%20page.html",
                     data=body, headers={"Content-Type": "text/html; charset=utf-8"})
    req.context["payload_signing_enabled"] = True
    S3SigV4Auth(creds, "s3", "auto").add_auth(req)
    print("PUT_AUTH", req.headers["Authorization"])
    print("PUT_SHA", hashlib.sha256(body).hexdigest()); print("PUT_DATE", req.headers["X-Amz-Date"])

    req2 = AWSRequest(method="GET", url="https://acct123.r2.cloudflarestorage.com/faz-sites/sites/abc/v1/index.html")
    S3SigV4QueryAuth(creds, "s3", "auto", expires=900).add_auth(req2)
    print("PRESIGN", req2.url)
