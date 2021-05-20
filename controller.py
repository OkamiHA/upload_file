import logging

import jwt
from webargs import fields, validate
from webargs.flaskparser import use_kwargs

from application.celery.edr_tasks import tasks as edr_tasks
from application.extensions import action_logger, parser
from application.services.all_services import edr_service
from application.services.mongo_service import CustomerEdrAgentDocument

logger = logging.getLogger("action")
action_logger.logger = logger


authorization_schema = {
    'authorization': fields.Str()
}

search_agents_schema = {
    'hostname': fields.Str(required=True),
    'since': fields.Integer(validate=validate.Range(min=0, max=1000), missing=0),
    'limit': fields.Integer(validate=validate.Range(min=0, max=100), missing=10)
}


def search_agents():
    kwargs = parser.parse(search_agents_schema)
    result = edr_service.search_agents(**kwargs)
    return result


def get_username(authorization):
    username = None
    try:
        if authorization:
            token = authorization.split(' ')[1]
            info = jwt.decode(token, verify=False, algorithms=['RS256'])
            username = info.get('identity')
    except Exception as e:
        logger.error("Get username from authorization got error: %s", e)
    return username


get_customer_agent_id_schema = {
    'customer_id': fields.Str(),
    '_from': fields.Integer(validate=validate.Range(min=0, max=1000), missing=0),
    '_size': fields.Integer(validate=validate.Range(min=0, max=1000), missing=10)
}


@use_kwargs(get_customer_agent_id_schema, location="query")
def get_customer_agent_id(**kwargs):
    _from = kwargs.pop("_from")
    _size = kwargs.pop("_size")
    result = CustomerEdrAgentDocument.objects(**kwargs).exclude("_id")
    count = result.count()
    result = result.skip(_from).limit(_size)
    data = [edr.to_mongo().to_dict() for edr in result]
    return {"data": data, "count": count}


agents_schema = {
    'agent_id': fields.Str(required=True),
    'hostname': fields.Str(required=True),
    'ip': fields.Str(),
    'device_type': fields.Str(validate=validate.OneOf(choices=["linux", "windows"])),
    'status': fields.Bool(default=True),
}
run_script_schema = {
    'customer_id': fields.Str(required=True),
    'campaign_name': fields.Str(required=True),
    'hunting_type': fields.Str(default="period"),
    'agents': fields.Nested(agents_schema, many=True)
}


@use_kwargs(authorization_schema, location="headers")
@use_kwargs(run_script_schema, location="json")
def run_script(**kwargs):
    count = 0
    authorization = kwargs.pop("authorization", None)
    username = get_username(authorization)
    if username:
        kwargs["username"] = username
    for agent in kwargs.get("agents"):
        set_agent = {"set__" + k: agent[k] for k in agent}
        edr_agents = CustomerEdrAgentDocument.objects(agent_id=agent["agent_id"], customer_id=kwargs["customer_id"])
        result = edr_agents.update_one(upsert=True, **set_agent)
        kwargs.update({"agent": agent})
        edr_tasks.run_script_one_agent.apply_async(kwargs=kwargs)
        count += result
    return {"count": count, "success": True}
