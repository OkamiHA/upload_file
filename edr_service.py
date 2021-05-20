import logging
import os
import ssl
import urllib.parse
from urllib.request import urlretrieve

import requests
from flask import abort, jsonify
from requests.packages.urllib3.exceptions import InsecureRequestWarning

from application.extensions import action_logger

ssl._create_default_https_context = ssl._create_unverified_context

logger = logging.getLogger("action")
action_logger.logger = logger

requests.packages.urllib3.disable_warnings(InsecureRequestWarning)


class EdrService(object):
    def __init__(self, edr_url, username, password):
        self.edr_url = edr_url
        self.login_url = urllib.parse.urljoin(edr_url, "/authentication/SignIn")
        self.info_agent_url = urllib.parse.urljoin(edr_url, "/agentManagement/QueryAgentInfoExtended")
        self.search_agent_url = urllib.parse.urljoin(edr_url, "/agentManagement/Search")
        self.send_request_get_file_url = urllib.parse.urljoin(edr_url, "/proxyHandler/SendRequestGetFile")
        self.get_file_result_url = urllib.parse.urljoin(edr_url, "/proxyHandler/GetFileResult")
        self.deploy_tool_url = urllib.parse.urljoin(edr_url, "/proxyHandler/DeployTool")
        self.get_tool_result_url = urllib.parse.urljoin(edr_url, "/proxyHandler/GetToolResult")
        self.access_token = None
        self.refresh_token = None
        self.username = username
        self.password = password

    def post(self, url, body, access_token=True):
        headers = {}
        if access_token and self.access_token:
            headers = {"Authorization": "Bearer " + self.access_token}
        response = requests.post(url, json=body, headers=headers, verify=False, timeout=30)
        if response.status_code == 200:
            data = response.json()
            return data
        logger.info("Request EDR %s response status_code: %s", url, response.status_code)
        return {"success": False, "status": response.status_code}
    
    def check_expired(self):
        body_test = {
            "query": {
                "compare": {
                    "field": "hostInfo.computerName",
                    "function": None,
                    "operator": "=",
                    "value": "ANM-CHUYENNT"
                }
            },
            "since": 0,
            "limit": 50
        }
        try:
            
    def login(self, username, password):
        try:
            body = {"username": username, "password": password}
            login_result = self.post(self.login_url, body, access_token=False)
            if login_result["success"]:
                self.access_token = login_result["access_token"]
                self.refresh_token = login_result["refresh_token"]
            return login_result
        except Exception as e:
            logger.info("Login EDR got error: %s", e)
        return {"success": False}

    def get_info_agents(self, agent_ids, infos=["hostInfo", "ip"]):
        try:
            body = {
                "agents": agent_ids,
                "infos": infos
            }
            return self.post(self.info_agent_url, body)
        except Exception as e:
            logger.info("Get info agents %s EDR got error: %s", agent_ids, e)
        return {"success": False}

    def search_agents(self, hostname, since=0, limit=100):
        try:
            body = {
                "since": since,
                "limit": limit
            }
            if hostname:
                body["query"] = {
                    "compare": {
                        "field": "hostInfo.computerName",
                        "function": None,
                        "operator": "~",
                        "value": hostname
                    }
                }
            return self.post(self.search_agent_url, body)
        except Exception as e:
            logger.info("Search agents by hostname %s EDR got error: %s", hostname, e)
        return abort(400, {"success": False, "message": 'Search agents by hostname "%s" EDR got error' % hostname})

    def send_request_get_file(self, agent_id, full_path):
        try:
            body = {
                "agent_id": agent_id,
                "full_path": full_path
            }
            return self.post(self.send_request_get_file_url, body)
        except Exception as e:
            logger.info("Send request get file %s EDR got error: %s", hostname, e)
        return {"success": False}

    def get_file_result(self, agent_id, request_id):
        try:
            body = {
                "agent_id": agent_id,
                "request_id": request_id
            }
            return self.post(self.get_file_result_url, body)
        except Exception as e:
            logger.info("Get file result %s EDR got error: %s", hostname, e)
        return {"success": False}

    def deploy_tool(self, agent_id_list, tool_id):
        try:
            body = {
                "agent_id_list": agent_id_list,
                "tool_id": tool_id
            }
            return self.post(self.deploy_tool_url, body)
        except Exception as e:
            logger.info("Deloy tool %s EDR got error: %s", hostname, e)
        return {"success": False}

    def get_tool_result(self, request_id):
        try:
            body = {
                "request_id": request_id
            }
            return self.post(self.get_tool_result_url, body)
        except Exception as e:
            logger.info("Get tool result %s EDR got error: %s", hostname, e)
        return {"success": False}

    def download_file(self, download_url, folder):
        try:
            r = requests.get(download_url, verify=False)
            if r.status_code == 200:
                info = r.headers["Content-Disposition"]
                file_name = info.split("=")[1].replace("/", "_")
                logger.info(file_name)
                path = os.path.join(folder, file_name)
                with open(path, 'wb') as f:
                    f.write(r.content)
                return {
                    'path': path,
                    'file_name': file_name,
                    'success': True
                }
            logger.info("Request EDR %s response status_code: %s", url, response.status_code)
            return {"success": False, "status": response.status_code}
        except Exception as e:
            logger.info("Download file %s from EDR got error: %s", download_url, e)
        return {"success": False}
