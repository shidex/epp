package model;

import javax.xml.bind.annotation.*;

@XmlRootElement(name = "epp", namespace = "urn:ietf:params:xml:ns:epp-1.0")
@XmlAccessorType(XmlAccessType.FIELD)
public class EppRequest {
    @XmlElement(name = "command", namespace = "urn:ietf:params:xml:ns:epp-1.0")
    public Command command;

    @XmlAccessorType(XmlAccessType.FIELD)
    public static class Command {
        @XmlElement(name = "login", namespace = "urn:ietf:params:xml:ns:epp-1.0")
        public Login login;

        @XmlElement(name = "greeting")
        public Greeting greeting;

        @XmlElement(name = "clTRID", namespace = "urn:ietf:params:xml:ns:epp-1.0")
        public String clTRID;
    }

    @XmlAccessorType(XmlAccessType.FIELD)
    public static class Login {
        @XmlElement(name = "clID", namespace = "urn:ietf:params:xml:ns:epp-1.0")
        public String clientId;

        @XmlElement(name = "pw", namespace = "urn:ietf:params:xml:ns:epp-1.0")
        public String password;

        @XmlElement(name = "newPW", namespace = "urn:ietf:params:xml:ns:epp-1.0")
        public String newPassword;
    }

    @XmlAccessorType(XmlAccessType.FIELD)
    public static class Greeting {
    }
}