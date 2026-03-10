package util;

import java.net.URI;

import javax.json.Json;
import javax.json.JsonObjectBuilder;

import org.springframework.http.HttpEntity;
import org.springframework.http.HttpHeaders;
import org.springframework.web.client.RestTemplate;
import dto.RegistrarDto;

public class EPPBackendApiUtil {

	public static String processEPPCommand (String apiURL,String eppRequest,String activeToken) {
		String result = "";
		URI externalAPIURL = null;
		try {
			externalAPIURL = new URI(apiURL);	
			
			RestTemplate restTemplate = new RestTemplate();
			
			HttpHeaders headers = new HttpHeaders();
			headers.set("authentication", activeToken);
			
			HttpEntity<String> request =new HttpEntity<String>(eppRequest,headers);
			
			
			result = restTemplate.postForObject(externalAPIURL, request,String.class);
		}
		catch (Exception e) {
			System.out.println("External API : " + externalAPIURL.toString());
			e.printStackTrace();
			result = "error";
		}
		
		
		return result;
	}
	public static String processEPPSessionCommand (String apiURL,String eppRequest,String activeToken) {
		String result = "";
		URI externalAPIURL = null;
		try {
			externalAPIURL = new URI(apiURL);	
			
			RestTemplate restTemplate = new RestTemplate();
			
			HttpHeaders headers = new HttpHeaders();
            headers.set("authentication", activeToken);
			
			HttpEntity<String> request =new HttpEntity<String>(eppRequest,headers);
			
			
			result = restTemplate.postForObject(externalAPIURL, request,String.class);
		}
		catch (Exception e) {
			//System.out.println("External API : " + externalAPIURL.toString());
			e.printStackTrace();
			result = "error";
		}
		
		
		return result;
	}
	public static RegistrarDto processAuthorization (String apiURL,String username,String password,String newPassword,String certificateHash,String ipAddress,String activeToken) {
		RegistrarDto result = null;
		URI externalAPIURL = null;
		try {
			externalAPIURL = new URI(apiURL);	
			
			RestTemplate restTemplate = new RestTemplate();
			
			String requestJson = "";
			
			HttpHeaders headers = new HttpHeaders();
            headers.set("authentication", activeToken);
			
			try {
				JsonObjectBuilder jsonBuilder = Json.createObjectBuilder();

				if (jsonBuilder != null) {
					if (username != null) {
						jsonBuilder.add(RegistrarDto.LABEL_EPPUSERNAME, username);
					}
					if (password != null) {
						jsonBuilder.add(RegistrarDto.LABEL_EPPPASSWORD, password);
					}
					if (newPassword != null) {
						jsonBuilder.add(RegistrarDto.LABEL_EPP_NEWPASSWORD, newPassword);
					}
					if (certificateHash != null) {
						jsonBuilder.add(RegistrarDto.LABEL_SERVER_CERTIFICATE_HASH, certificateHash);
					}
					if (ipAddress != null) {
						jsonBuilder.add(RegistrarDto.LABEL_IP_ADDRESS, ipAddress);
					}
					
					requestJson = jsonBuilder.build().toString();
				}
			}
			catch (Exception e) {
				e.printStackTrace();
			}
			
			HttpEntity<String> request =new HttpEntity<String>(requestJson,headers);
			//System.out.println("external api url : " + externalAPIURL);
			
			String response = restTemplate.postForObject(externalAPIURL, request,String.class);
			
			result = new RegistrarDto(response);
			
		}
		catch (Exception e) {
			//System.out.println("Exception External API : " + externalAPIURL.toString());
			e.printStackTrace();
			result = new RegistrarDto();
			result.setResponseCode("01");
			result.setResponseDescription("Failed Registrar Authentication");
		}
		
		
		return result;
	}
	public static RegistrarDto processLogout (String apiURL,String username,String activeToken) {
		RegistrarDto result = null;
		URI externalAPIURL = null;
		try {
			externalAPIURL = new URI(apiURL);	
			
			RestTemplate restTemplate = new RestTemplate();
			
			String requestJson = "";
			
			HttpHeaders headers = new HttpHeaders();
            headers.set("authentication", activeToken);
			
			try {
				JsonObjectBuilder jsonBuilder = Json.createObjectBuilder();

				if (jsonBuilder != null) {
					if (username != null) {
						jsonBuilder.add(RegistrarDto.LABEL_EPPUSERNAME, username);
					}
					
					
					requestJson = jsonBuilder.build().toString();
				}
			}
			catch (Exception e) {
				e.printStackTrace();
			}
			
			HttpEntity<String> request =new HttpEntity<String>(requestJson,headers);
			
			
			String response = restTemplate.postForObject(externalAPIURL, request,String.class);
			
			result = new RegistrarDto(response);
			
		}
		catch (Exception e) {
			//System.out.println("External API : " + externalAPIURL.toString());
			e.printStackTrace();
			result = new RegistrarDto();
			result.setResponseCode("01");
			result.setResponseDescription("Failed Registrar Authentication");
		}
		
		
		return result;
	}
}
