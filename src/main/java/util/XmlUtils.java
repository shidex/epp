package util;

import model.EppRequest;
import util.ConfigLoader;

import javax.xml.bind.*;
import java.io.*;
import java.time.Instant;
import java.time.format.DateTimeFormatter;
import javax.xml.XMLConstants;
import javax.xml.bind.*;
import javax.xml.validation.Schema;
import javax.xml.validation.SchemaFactory;

public class XmlUtils {
    private static final JAXBContext context;
    private static final Schema schema;

    static {
        try {
            context = JAXBContext.newInstance(EppRequest.class);
            SchemaFactory sf = SchemaFactory.newInstance(XMLConstants.W3C_XML_SCHEMA_NS_URI);
            schema = sf.newSchema(XmlUtils.class.getResource("/xsd/epp-1.0.xsd")); // pastikan file tersedia di /resources/xsd/
            if (schema == null) {
                throw new FileNotFoundException("XSD file not found in /resources/xsd/epp-1.0.xsd");
            }
        } catch (Exception e) {
            throw new RuntimeException("Failed to initialize JAXB context or schema", e);
        }
    }

    public static EppRequest parseXml(String xml) {
        try {
            Unmarshaller unmarshaller = context.createUnmarshaller();
            boolean enableValidation = ConfigLoader.get("enable.validation").equalsIgnoreCase("true");
            if (enableValidation) {
                try {
                    unmarshaller.setSchema(schema);
                    unmarshaller.setEventHandler(event -> {
                    System.err.println("[XML VALIDATION WARNING]");
                    System.err.println("  - Message: " + event.getMessage());
                    System.err.println("  - Line: " + event.getLocator().getLineNumber());
                    System.err.println("  - Column: " + event.getLocator().getColumnNumber());
                    return false; // stop on first validation error
                });
                } catch (Exception e) {
                    System.err.println("[WARN] Schema validation disabled due to error: " + e.getMessage());
                }
            } else {
                // No schema attached
                System.out.println("[INFO] XML validation disabled via config.");
            }
            return (EppRequest) unmarshaller.unmarshal(new StringReader(xml));
        } catch (Exception e) {
            throw new RuntimeException("Invalid XML", e);
        }
    }

    public static String buildLoginResponse(String sessionId, String clTRID) {
         String login = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"no\"?>"
				+ "<epp xmlns=\"urn:ietf:params:xml:ns:epp-1.0\">"
				+ "<response><result code=\"1000\">"
                + "<msg>Command completed successfully</msg></result>" 
                + "<trID><clTRID>" + clTRID + "</clTRID><svTRID>" + sessionId + "</svTRID></trID></response></epp>";
        
        return login;
    }

    public static String buildAuthFailResponse(String sessionId) {
        String loginFail = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"no\"?>"
				+ "<epp xmlns=\"urn:ietf:params:xml:ns:epp-1.0\">"
				+ "<response><result code=\"2200\">"
                + "<msg>Authentication failed</msg></result>"
                + "<trID><svTRID>" + sessionId + "</svTRID></trID></response></epp>";

        return loginFail;
    }

    public static String buildGreetingResponse() {
        String svDate = DateTimeFormatter.ISO_INSTANT.format(Instant.now());

        return "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"no\"?>"
            + "<epp xmlns=\"urn:ietf:params:xml:ns:epp-1.0\" "
            + "xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\" "
            + "xsi:schemaLocation=\"urn:ietf:params:xml:ns:epp-1.0 epp-1.0.xsd\">"
            + "<greeting>"
            + "<svID>epp.adg.id</svID>"
            + "<svDate>" + svDate + "</svDate>"
            + "<svcMenu>"
            + "<version>1.0</version>"
            + "<lang>en</lang>"
            + "<objURI>urn:ietf:params:xml:ns:domain-1.0</objURI>"
            + "<objURI>urn:ietf:params:xml:ns:contact-1.0</objURI>"
            + "<objURI>urn:ietf:params:xml:ns:host-1.0</objURI>"
            + "<svcExtension>"
            + "<extURI>urn:ietf:params:xml:ns:secDNS-1.1</extURI>"
            + "<extURI>urn:ietf:params:xml:ns:launch-1.0</extURI>"
            + "<extURI>urn:ietf:params:xml:ns:rgp-1.0</extURI>"
            + "</svcExtension>"
            + "</svcMenu>"
            + "<dcp>"
            + "<access><all/></access>"
            + "<statement>"
            + "<purpose><admin/><prov/></purpose>"
            + "<recipient><ours/><public/></recipient>"
            + "<retention><stated/></retention>"
            + "</statement>"
            + "</dcp>"
            + "</greeting>"
            + "</epp>";
    }

    public static String buildRateLimitResponse(String sessionId) {
        String rateLimit = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"no\"?>"
				+ "<epp xmlns=\"urn:ietf:params:xml:ns:epp-1.0\">"
				+ "<response><result code=\"2400\">"
                + "<msg>Rate limit exceeded; please retry later</msg></result>"
                + "<trID><svTRID>" + sessionId + "</svTRID></trID></response></epp>";

        return rateLimit;
    }

    public static String buildErrorResponse(String message) {
        return "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"no\"?>" +
               "<epp xmlns=\"urn:ietf:params:xml:ns:epp-1.0\">" +
               "<response><result code=\"2004\">" +
               "<msg>" + message + "</msg></result></response></epp>";
    }

    public static String buildLogoutResponse(String svTRID, String clTRID) {
        return "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"no\"?>" +
               "<epp xmlns=\"urn:ietf:params:xml:ns:epp-1.0\">" +
               "<response><result code=\"1500\">" +
               "<msg>Command completed successfully; ending session</msg>" +
               "</result><trID><clTRID>" + clTRID + "</clTRID><svTRID>" + svTRID + "</svTRID></trID></response></epp>";
    }

    /** In the future, <resData> can be populated with polling details like domain transfer messages. */
    public static String buildPollResponse() {
        return "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"no\"?>" +
               "<epp xmlns=\"urn:ietf:params:xml:ns:epp-1.0\">" +
               "<response><result code=\"1300\">" +
               "<msg>No messages</msg></result>" +
               "<msgQ count=\"0\" /><resData/></response></epp>";
    }

    public static String buildPolicyErrorResponse(String message) {
    return "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"no\"?>" 
         + "<epp xmlns=\"urn:ietf:params:xml:ns:epp-1.0\">"
         + "<response>"
         + "<result code=\"2306\">"
         + "<msg>" + message + "</msg>"
         + "</result>"
         + "</response>"
         + "</epp>";
}
}
