import logging
import os
import tempfile
from base64 import b64decode

from celery.result import allow_join_result
from celery.utils.log import get_logger

from application.celery.celery import celery_app
from application.services.edr_service import EdrService
from application.services.mongo_service import HistoryHuntingEdrDocument, MongoService
from application.services.zip_service import ZipperService
from application.services.external_service import UploadHuntingService
from application.utils import Config
from base64 import b64decode

logger = get_logger(__name__)
logger.setLevel(logging.INFO)

edr_service = EdrService(Config.get_config("edr_url"), Config.get_config("edr_username"), Config.get_config("edr_password"))
upload_hunting_service = UploadHuntingService(Config.get_config("upload_hunting_url"))
login_status = edr_service.login()
map_tool_id = Config.get_config("map_tool_id", {})
map_tool_name = Config.get_config("map_tool_name", {})


@celery_app.task
def run_script_one_agent(customer_id, campaign_name, hunting_type, agent, **kwargs):
    try:
        logger.debug(locals())
        device_type = agent.get("device_type")
        script_tool_id = map_tool_id.get(device_type)
        script_tool_name = map_tool_name.get(device_type)
        agent_id = agent["agent_id"]
        hostname = agent["hostname"]
        if script_tool_id is None:
            logger.error("Not found tool_id of device_type: %s", device_type)
        section_date = MongoService.get_section_date(customer_id, campaign_name)
        section_id = MongoService.get_section_id(customer_id, campaign_name)
        temp_dir = tempfile.mkdtemp()
        with allow_join_result():
            deploy_tool_task = deploy_tool.delay(agent_id, script_tool_id, script_tool_name, hostname)
            deploy_tool_result = deploy_tool_task.get()
            if deploy_tool_result["status"] == "failed":
                message_error = "Deloy script on %s (%s) got error: %s" % (
                    hostname, agent_id, deploy_tool_result["message"])
                raise Exception(message_error)
            request_id = deploy_tool_result["request_id"]
            history_edr = HistoryHuntingEdrDocument(customer_id=customer_id, campaign_name=campaign_name,
                                                    hunting_type=hunting_type, agent=agent, is_running=True,
                                                    request_id=request_id)
            history_edr.save()
            get_tool_result_task = get_tool_result.delay(agent_id, deploy_tool_result["request_id"],
                                                         script_tool_name, hostname)
            tool_result = get_tool_result_task.get()
            if deploy_tool_result["status"] == "failed":
                message_error = "Deloy script on %s (%s) got error: %s" % (
                    hostname, agent_id, tool_result["message"])
                raise Exception(message_error)
            history_edr.update(is_running=False)
            data = b64decode(tool_result["data"]).decode()
            full_path = data.split("\n")[-2]
            logger.debug("full_path: %s", full_path)
            logger.debug(os.path.splitext(os.path.basename(full_path)))
            object_folder_name = os.path.splitext(os.path.basename(full_path))[0]
            section_folder_name = "{customer_id}_{section_date}".format(customer_id=customer_id,
                                                                        section_date=section_date)
            object_folder = os.path.join(temp_dir, section_folder_name, section_folder_name,
                                         object_folder_name, object_folder_name)
            section_folder = os.path.join(temp_dir, section_folder_name)
            send_request_get_file_task = send_request_get_file.delay(agent_id, full_path, hostname)
            send_request_get_file_result = send_request_get_file_task.get()
            if not send_request_get_file_result["success"]:
                message_error = "Send request download file on %s (%s) got error: %s" % (
                    hostname, agent_id, send_request_get_file_task["reason"])
                raise Exception(message_error)
            get_file_task = get_file_result.delay(agent_id, full_path, send_request_get_file_result["req_id"])
            file_result = get_file_task.get()
            download_url = file_result["download_url"]
            history_edr.update(is_downloading=True)
            download_file_task = download_file.delay(agent_id, full_path, download_url, temp_dir)
            download_file_result = download_file_task.get()
            if not download_file_result["success"]:
                message_error = "Download file url %s on %s (%s) got error: %s" % (download_url, hostname, agent_id,
                                                                                   download_file_result["message"])
                raise Exception(message_error)
            download_file_path = download_file_result["path"]
            history_edr.update(is_downloading=False)
            pwd = ZipperService.extract_password(download_file_path)
            result, folder = ZipperService.unzip(download_file_path, pwd=pwd, remove_file=True)
            logger.info("Unzip %d files from file %s to folder %s", len(result), download_file_path, folder)
            unziped_files = ZipperService.find_file_extension(folder, "tar")
            for unziped_file in unziped_files:
                result, folder = ZipperService.unzip(unziped_file, parent_folder=object_folder, remove_file=True)
                logger.info("Unzip %d files from %s to %s", len(result), unziped_file, folder)
                object_zip = ZipperService.zip_folder(os.path.dirname(object_folder),
                                                      object_folder_name + ".zip", remove_folder=True)
                logger.info("Zip folder %s to %s in %s", object_folder, object_zip, section_folder)
                section_zip = ZipperService.zip_folder(section_folder, section_folder_name + ".zip", remove_folder=True)
                logger.info("Zip folder %s to %s in %s", section_folder, section_zip, temp_dir)
                upload_hunting_task = upload_hunting.delay(customer_id, campaign_name,
                                                           hunting_type, section_zip, hostname=agent["hostname"],
                                                           section_id=section_id, authorization=kwargs.get("authorization"))
                upload_hunting_result = upload_hunting_task.get()
                if not upload_hunting_result["success"]:
                    logger.info("Upload %s contains %s got: %s", section_zip,
                                hostname, upload_hunting_result)
                    raise Exception("Upload file %s contains hostname %s got error: %s" %
                                    (section_zip, hostname, upload_hunting_result["message"]))
                history_edr.update(uploaded=True)
    except Exception as e:
        logger.error("Run script on agent %s got error %s", agent, e)
        logger.error("Push notification")


@celery_app.task(bind=True, autoretry_for=(Exception,), max_retries=5, default_retry_delay=5)
def deploy_tool(self, agent_id, tool_id, tool_name="", hostname=""):
    try:
        deploy_tool_result = edr_service.deploy_tool([agent_id], tool_id)
        logger.debug("deploy_tool_result %s (%s) %s (%s): %s", tool_name,
                     tool_id, hostname, agent_id, deploy_tool_result)
        return deploy_tool_result
    except Exception as e:
        if self.request.retries == self.max_retries:
            raise Exception("Get tool result %s (%s) of agent %s (%s) got error: %s" %
                            (tool_name, tool_id, hostname, agent_id, e))
        logger.error(e)
        self.retry(exc=e, countdown=self.default_retry_delay * (self.request.retries + 1))


@celery_app.task(bind=True, autoretry_for=(Exception,), max_retries=20, default_retry_delay=5)
def get_tool_result(self, agent_id, request_id, tool_name="", hostname=""):
    try:
        tool_result = edr_service.get_tool_result(request_id)
        logger.debug("tool_result %s (%s): %s", tool_name, request_id, tool_result)
        if tool_result['result_list'][0]["status"] == "pending":
            raise Exception("Request deploy_tool %s (%s) of agent %s (%s) is pending" %
                            (tool_name, request_id, hostname, agent_id))
        return tool_result['result_list'][0]
    except Exception as e:
        if self.request.retries == self.max_retries:
            raise Exception("Can not get tool %s result request %s of agent %s (%s) expired" %
                            (tool_name, request_id, hostname, agent_id))
        logger.error(e)
        self.retry(exc=e, countdown=self.default_retry_delay * (self.request.retries + 1))


@celery_app.task(bind=True, autoretry_for=(Exception,), max_retries=15, default_retry_delay=3)
def send_request_get_file(self, agent_id, full_path, hostname=""):
    try:
        send_request_result = edr_service.send_request_get_file(agent_id, full_path)
        logger.debug("send_request_get_file_result %s: %s", full_path, send_request_result)
        return send_request_result
    except Exception as e:
        if self.request.retries == self.max_retries:
            raise Exception("Send request get file %s of agent %s (%s) got error." %
                            (full_path, hostname, agent_id, e))
        logger.error(e)
        self.retry(exc=e, countdown=self.default_retry_delay * (self.request.retries + 1))


@celery_app.task(bind=True, max_retries=20, default_retry_delay=5)
def get_file_result(self, agent_id, full_path, request_id, hostname=""):
    try:
        get_file_result = edr_service.get_file_result(agent_id, request_id)
        logger.debug("get_file_result: %s", get_file_result)
        if get_file_result["status"] == "pending":
            raise Exception("Request download_file %s (%s) of agent %s (%s) is pending" %
                            (full_path, request_id, hostname, agent_id))
        return get_file_result
    except Exception as e:
        if self.request.retries == self.max_retries:
            raise Exception("Download_file %s result %s of agent %s (%s) expired" %
                            (full_path, request_id, hostname, agent_id))
        logger.info(e)
        self.retry(exc=e, countdown=self.default_retry_delay * (self.request.retries + 1))


@celery_app.task(bind=True, max_retries=5, default_retry_delay=5)
def download_file(self, agent_id, full_path, download_url, folder):
    try:
        download_file_result = edr_service.download_file(download_url, folder)
        logger.debug("download_file_result: %s", download_file_result)
        return download_file_result
    except Exception as e:
        if self.request.retries == self.max_retries:
            raise Exception("Can not download_file %s (%s) of agent %s (%s) got error: %s" %
                            (full_path, download_url, hostname, agent_id, e))
        logger.error(e)
        self.retry(exc=e, countdown=self.default_retry_delay * (self.request.retries + 1))


@celery_app.task(bind=True, max_retries=3, default_retry_delay=5)
def upload_hunting(self, customer_id, campaign_name, hunting_type, filezip, authorization=None, hostname=None, section_id=None):
    try:
        upload_hunting_result = upload_hunting_service.upload(customer_id, campaign_name, hunting_type,
                                                              filezip, authorization, section_id=section_id)
        logger.debug("upload_hunting_result %s: %s", filezip, upload_hunting_result)
        return upload_hunting_result
    except Exception as e:
        if self.request.retries == self.max_retries:
            raise Exception("Can not upload file %s of hostname %s after %d times" %
                            (filezip, hostname, self.max_retries))
        logger.error(e)
        self.retry(exc=e, countdown=self.default_retry_delay * (self.request.retries + 1))
