from typing import NewType, Optional, List, Any

MessageType = NewType("MessageType", str)

MESSAGE_TYPE_CONNECT: MessageType = MessageType("connect") # Connect
MESSAGE_TYPE_HANDSHAKE: MessageType = MessageType("handshake") # Handshake
MESSAGE_TYPE_ACK: MessageType = MessageType("ack") # Acknowledgement
MESSAGE_TYPE_INITIALIZE: MessageType = MessageType("initialize") # Initialize
MESSAGE_TYPE_VALIDATION: MessageType = MessageType("validation") # Validation

MESSAGE_TYPE_CONNECTION: MessageType = MessageType("connection") # Connection
MESSAGE_TYPE_COMMAND: MessageType = MessageType("command") # Command
MESSAGE_TYPE_PING: MessageType = MessageType("ping") # Ping

class Message:
    """Message class to represent a message sent between the client and server."""
    def __init__(self,
                 type: MessageType,
                 data: Optional[str] = None,
                 arguments: Optional[List[str]] = None,
                 json_data: Optional[Any] = None,
                 tags: Optional[List[str]] = None,
                 ) -> None:
        self.type: MessageType = type
        self.data: Optional[str] = data
        self.arguments: Optional[List[str]] = arguments
        self.json_data: Optional[Any] = json_data
        self.tags: Optional[List[str]] = tags

    def to_dict(self) -> dict:
        """Convert the Message object to a dictionary for JSON serialization."""
        return {
            "type": self.type,
            "data": self.data,
            "arguments": self.arguments,
            "json_data": self.json_data,
            "tags": self.tags,
        }

    def __repr__(self) -> str:
        return (
            f"Message(type={self.type}, "
            f"data={self.data}, "
            f"arguments={self.arguments}, "
            f"json_data={self.json_data}, "
            f"tags={self.tags})"
        )
