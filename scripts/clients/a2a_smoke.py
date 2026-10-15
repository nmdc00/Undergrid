"""Independent a2a-sdk==1.2.2 transport proof against a local fixture server."""
import asyncio
import json
import sys
import uuid

import httpx
from google.protobuf.json_format import ParseDict
from a2a.client.transports.rest import RestTransport
from a2a.types.a2a_pb2 import AgentCard, GetTaskRequest, ListTasksRequest, SendMessageRequest


async def main():
    config = json.load(sys.stdin)
    async with httpx.AsyncClient(headers={
        'Authorization': 'Bearer ' + config['token'],
        'A2A-Version': '1.0',
        'A2A-Extensions': config['extension'],
    }) as http:
        card_response = await http.get(config['origin'] + '/.well-known/agent-card.json')
        card_response.raise_for_status()
        card = ParseDict(card_response.json(), AgentCard())
        assert not card.capabilities.streaming
        transport = RestTransport(http, card, config['origin'] + '/a2a')
        request = ParseDict({'message': {
            'messageId': str(uuid.uuid4()), 'role': 'ROLE_USER', 'parts': [{'data': {
                'version': '1', 'operation': 'intention_create',
                'arguments': {'idempotency_key': str(uuid.uuid4()), 'body': config['intention']},
            }}],
        }}, SendMessageRequest())
        first = await transport.send_message(request)
        assert first.HasField('task'), first
        second = await transport.send_message(request)
        assert second.task.id == first.task.id
        recovered = await transport.get_task(GetTaskRequest(id=first.task.id))
        assert recovered.id == first.task.id
        assert not recovered.history
        listed = await transport.list_tasks(ListTasksRequest(page_size=1))
        assert len(listed.tasks) == 1
        print(json.dumps({'receipt_id': first.task.id, 'replay': True, 'recovered': True}))


asyncio.run(main())
