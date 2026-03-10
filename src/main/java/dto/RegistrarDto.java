package dto;

import java.io.StringReader;
import java.util.List;

import javax.json.Json;
import javax.json.JsonObject;
import javax.json.JsonObjectBuilder;
import javax.json.JsonReader;

public class RegistrarDto implements java.io.Serializable {
	private static final long serialVersionUID = 1L;

	public static final int STATUS_ACTIVE = 1;
	public static final int STATUS_PENDING = 0;
	public static final int STATUS_BLOCKED = -1;
	
	
	public static final String LABEL_RESPONSE_CODE = "responseCode";
	public static final String LABEL_RESPONSE_DESCRIPTION = "responseDesc";
	public static final String LABEL_RESPONSE_TRANSACTION_ID = "responseTrxId";

	// Fields
	public static final String LABEL_REGISTRARID = "registrarId";
	public static final String LABEL_REGISTRARUNIQUECODE = "registrarUniqueCode";
	public static final String LABEL_REGISTRARGUID = "registrarGuid";
	public static final String LABEL_ORGANIZATIONNAME = "organizationName";
	public static final String LABEL_REGISTRAR_NAME = "registrarName";
	public static final String LABEL_ORGANIZATIONADDRESS = "organizationAddress";
	public static final String LABEL_ADDRESS1 = "address1";
	public static final String LABEL_ADDRESS2 = "address2";
	public static final String LABEL_ADDRESS3 = "address3";
	public static final String LABEL_POSTALCODE = "postalCode";
	public static final String LABEL_CITY = "city";
	public static final String LABEL_PROVINCE = "province";
	public static final String LABEL_COUNTRY = "country";
	public static final String LABEL_TELEPHONE = "telephone";
	public static final String LABEL_FAXIMILE = "faximile";
	public static final String LABEL_EMAIL = "email";
	public static final String LABEL_EMAILOPERATION = "emailOperation";
	public static final String LABEL_EMAILSECURITY = "emailSecurity";
	public static final String LABEL_EMAILBILLING = "emailBilling";
	public static final String LABEL_EMAILLEGAL = "emailLegal";
	public static final String LABEL_EMAILTRANSFER = "emailTransfer";
	public static final String LABEL_EMAILCREDITLIMIT = "emailCreditLimit";
	public static final String LABEL_EMAILDELETION = "emailDeletion";
	public static final String LABEL_EMAILMARKETING = "emailMarketing";
	public static final String LABEL_WEBSITE = "website";
	public static final String LABEL_LEVEL = "level";
	public static final String LABEL_MANAGER = "manager";
	public static final String LABEL_CURRENCY = "currency";
	public static final String LABEL_CREDITLIMIT = "creditLimit";
	public static final String LABEL_EMERGENCY_CREDITLIMIT = "emergencyCreditLimit";
	public static final String LABEL_RENEWTYPE = "renewType";
	public static final String LABEL_BILLINGMODEL = "billingModel";
	public static final String LABEL_BILLINGINTERVAL = "billingInterval";
	public static final String LABEL_REGISTRARSTATUS = "registrarStatus";
	public static final String LABEL_REGISTRARRATINGREVIEW = "registrarRatingReview";
	public static final String LABEL_REGISTRATIONDATE = "registrationDate";
	public static final String LABEL_EXPIREDATE = "expireDate";
	public static final String LABEL_RENEWALDATE = "renewalDate";
	public static final String LABEL_DIRECTORNAME = "directorName";
	public static final String LABEL_NPWP = "npwp";
	public static final String LABEL_SIUP = "siup";
	public static final String LABEL_SKPENDIRIAN = "skPendirian";
	public static final String LABEL_CREATEDTIME = "createdTime";
	public static final String LABEL_CREATEDBY = "createdBy";
	public static final String LABEL_MODIFIEDTIME = "modifiedTime";
	public static final String LABEL_MODIFIEDBY = "modifiedBy";
	public static final String LABEL_DELETEDTIME = "deletedTime";
	public static final String LABEL_DELETEDBY = "deletedBy";
	public static final String LABEL_DELETEDSTATUS = "deletedStatus";
	public static final String LABEL_EPPUSERNAME = "eppUsername";
	public static final String LABEL_EPPPASSWORD = "eppPassword";
	public static final String LABEL_EPP_NEWPASSWORD = "eppNewPassword";
	public static final String LABEL_EPPSTATUS = "eppStatus";
	public static final String LABEL_EPP_SESSION_TOKEN = "eppSessionToken";
	public static final String LABEL_REASON = "reason";
	public static final String LABEL_TAX_PERCENTAGE = "taxPercentage";
	public static final String LABEL_CURRENCY_NAME = "currencyName";
	public static final String LABEL_PRICING_LEVEL = "pricingLevelId";
	public static final String LABEL_VIRTUAL_ACCOUNT_NUMBER = "virtualAccountNumber";
	public static final String LABEL_SERVER_CERTIFICATE_HASH = "serverCertificateHash";
	public static final String LABEL_IP_ADDRESS = "ipAddress";
	public static final String LABEL_APIUSERNAME = "apiUsername";
	public static final String LABEL_APIPASSWORD = "apiPassword";
	public static final String LABEL_API_SESSION_TOKEN = "apiSessionToken";
	public static final String LABEL_APISTATUS = "apiStatus";
	public static final String LABEL_ACTION_TYPE = "actionType";


	// Fields
	private String registrarId;
	private String registrarUniqueCode;
	private String registrarGuid;
	private String registrarName;
	private String organizationName;
	private String organizationAddress;
	private String address1;
	private String address2;
	private String address3;
	private String postalCode;
	private String city;
	private String province;
	private String country;
	private String telephone;
	private String faximile;
	private String email;
	private String emailOperation;
	private String emailSecurity;
	private String emailBilling;
	private String emailLegal;
	private String emailTransfer;
	private String emailCreditLimit;
	private String emailDeletion;
	private String emailMarketing;
	private String website;
	private String level;
	private String manager;
	private String currency;
	private String creditLimit;
	private String emergencyCreditLimit;
	private String renewType;
	private String billingModel;
	private String billingInterval;
	private String registrarStatus;
	private String registrarRatingReview;
	private String registrationDate;
	private String expireDate;
	private String renewalDate;
	private String directorName;
	private String npwp;
	private String siup;
	private String skPendirian;
	private String createdTime;
	private String createdBy;
	private String modifiedTime;
	private String modifiedBy;
	private String deletedTime;
	private String deletedBy;
	private String deletedStatus;
	private String eppUsername;
	private String eppPassword;
	private String eppNewPassword;
	private String eppStatus;
	private String eppSessionToken;
	private String reason;
	private String currencyName;
	private String taxPercentage;
	private String pricingLevelId;
	private String virtualAccountNumber;
	private String serverCertificateHash;
	private String ipAddress;
	private String apiUsername;
	private String apiPassword;
	private String apiStatus;
	private String apiSessionToken;
	private String actionType;
	private String responseCode;
	private String responseDescription;
	private String responseTransactionId;
	
	private List<String> domainList;

	public RegistrarDto() {
	}

	public RegistrarDto(String json) {
		if (json != null) {
			StringReader reader = new StringReader(json);

			JsonReader jsonReader = Json.createReader(reader);
			JsonObject responseObject = jsonReader.readObject();
			jsonReader.close();

			if (responseObject.containsKey(LABEL_REGISTRARID)) {
				registrarId = responseObject.getString(LABEL_REGISTRARID);
			}
			if (responseObject.containsKey(LABEL_REGISTRAR_NAME)) {
				registrarName = responseObject.getString(LABEL_REGISTRAR_NAME);
			}
			if (responseObject.containsKey(LABEL_EMERGENCY_CREDITLIMIT)) {
				emergencyCreditLimit = responseObject.getString(LABEL_EMERGENCY_CREDITLIMIT);
			}
			if (responseObject.containsKey(LABEL_REGISTRARUNIQUECODE)) {
				registrarUniqueCode = responseObject.getString(LABEL_REGISTRARUNIQUECODE);
			}
			if (responseObject.containsKey(LABEL_REGISTRARGUID)) {
				registrarGuid = responseObject.getString(LABEL_REGISTRARGUID);
			}
			if (responseObject.containsKey(LABEL_ORGANIZATIONNAME)) {
				organizationName = responseObject.getString(LABEL_ORGANIZATIONNAME);
			}
			if (responseObject.containsKey(LABEL_ORGANIZATIONADDRESS)) {
				organizationAddress = responseObject.getString(LABEL_ORGANIZATIONADDRESS);
			}
			if (responseObject.containsKey(LABEL_ADDRESS1)) {
				address1 = responseObject.getString(LABEL_ADDRESS1);
			}
			if (responseObject.containsKey(LABEL_ADDRESS2)) {
				address2 = responseObject.getString(LABEL_ADDRESS2);
			}
			if (responseObject.containsKey(LABEL_ADDRESS3)) {
				address3 = responseObject.getString(LABEL_ADDRESS3);
			}
			if (responseObject.containsKey(LABEL_POSTALCODE)) {
				postalCode = responseObject.getString(LABEL_POSTALCODE);
			}
			if (responseObject.containsKey(LABEL_CITY)) {
				city = responseObject.getString(LABEL_CITY);
			}
			if (responseObject.containsKey(LABEL_PROVINCE)) {
				province = responseObject.getString(LABEL_PROVINCE);
			}
			if (responseObject.containsKey(LABEL_COUNTRY)) {
				country = responseObject.getString(LABEL_COUNTRY);
			}
			if (responseObject.containsKey(LABEL_TELEPHONE)) {
				telephone = responseObject.getString(LABEL_TELEPHONE);
			}
			if (responseObject.containsKey(LABEL_FAXIMILE)) {
				faximile = responseObject.getString(LABEL_FAXIMILE);
			}
			if (responseObject.containsKey(LABEL_EMAIL)) {
				email = responseObject.getString(LABEL_EMAIL);
			}
			if (responseObject.containsKey(LABEL_EMAILOPERATION)) {
				emailOperation = responseObject.getString(LABEL_EMAILOPERATION);
			}
			if (responseObject.containsKey(LABEL_EMAILSECURITY)) {
				emailSecurity = responseObject.getString(LABEL_EMAILSECURITY);
			}
			if (responseObject.containsKey(LABEL_EMAILBILLING)) {
				emailBilling = responseObject.getString(LABEL_EMAILBILLING);
			}
			if (responseObject.containsKey(LABEL_EMAILLEGAL)) {
				emailLegal = responseObject.getString(LABEL_EMAILLEGAL);
			}
			if (responseObject.containsKey(LABEL_EMAILTRANSFER)) {
				emailTransfer = responseObject.getString(LABEL_EMAILTRANSFER);
			}
			if (responseObject.containsKey(LABEL_EMAILCREDITLIMIT)) {
				emailCreditLimit = responseObject.getString(LABEL_EMAILCREDITLIMIT);
			}
			if (responseObject.containsKey(LABEL_EMAILDELETION)) {
				emailDeletion = responseObject.getString(LABEL_EMAILDELETION);
			}
			if (responseObject.containsKey(LABEL_EMAILMARKETING)) {
				emailMarketing = responseObject.getString(LABEL_EMAILMARKETING);
			}
			if (responseObject.containsKey(LABEL_WEBSITE)) {
				website = responseObject.getString(LABEL_WEBSITE);
			}
			if (responseObject.containsKey(LABEL_LEVEL)) {
				level = responseObject.getString(LABEL_LEVEL);
			}
			if (responseObject.containsKey(LABEL_MANAGER)) {
				manager = responseObject.getString(LABEL_MANAGER);
			}
			if (responseObject.containsKey(LABEL_CURRENCY)) {
				currency = responseObject.getString(LABEL_CURRENCY);
			}
			if (responseObject.containsKey(LABEL_CREDITLIMIT)) {
				creditLimit = responseObject.getString(LABEL_CREDITLIMIT);
			}
			if (responseObject.containsKey(LABEL_RENEWTYPE)) {
				renewType = responseObject.getString(LABEL_RENEWTYPE);
			}
			if (responseObject.containsKey(LABEL_BILLINGMODEL)) {
				billingModel = responseObject.getString(LABEL_BILLINGMODEL);
			}
			if (responseObject.containsKey(LABEL_BILLINGINTERVAL)) {
				billingInterval = responseObject.getString(LABEL_BILLINGINTERVAL);
			}
			if (responseObject.containsKey(LABEL_REGISTRARSTATUS)) {
				registrarStatus = responseObject.getString(LABEL_REGISTRARSTATUS);
			}
			if (responseObject.containsKey(LABEL_REGISTRARRATINGREVIEW)) {
				registrarRatingReview = responseObject.getString(LABEL_REGISTRARRATINGREVIEW);
			}
			if (responseObject.containsKey(LABEL_REGISTRATIONDATE)) {
				registrationDate = responseObject.getString(LABEL_REGISTRATIONDATE);
			}
			if (responseObject.containsKey(LABEL_EXPIREDATE)) {
				expireDate = responseObject.getString(LABEL_EXPIREDATE);
			}
			if (responseObject.containsKey(LABEL_RENEWALDATE)) {
				renewalDate = responseObject.getString(LABEL_RENEWALDATE);
			}
			if (responseObject.containsKey(LABEL_DIRECTORNAME)) {
				directorName = responseObject.getString(LABEL_DIRECTORNAME);
			}
			if (responseObject.containsKey(LABEL_NPWP)) {
				npwp = responseObject.getString(LABEL_NPWP);
			}
			if (responseObject.containsKey(LABEL_SIUP)) {
				siup = responseObject.getString(LABEL_SIUP);
			}
			if (responseObject.containsKey(LABEL_SKPENDIRIAN)) {
				skPendirian = responseObject.getString(LABEL_SKPENDIRIAN);
			}
			if (responseObject.containsKey(LABEL_CREATEDTIME)) {
				createdTime = responseObject.getString(LABEL_CREATEDTIME);
			}
			if (responseObject.containsKey(LABEL_CREATEDBY)) {
				createdBy = responseObject.getString(LABEL_CREATEDBY);
			}
			if (responseObject.containsKey(LABEL_MODIFIEDTIME)) {
				modifiedTime = responseObject.getString(LABEL_MODIFIEDTIME);
			}
			if (responseObject.containsKey(LABEL_MODIFIEDBY)) {
				modifiedBy = responseObject.getString(LABEL_MODIFIEDBY);
			}
			if (responseObject.containsKey(LABEL_DELETEDTIME)) {
				deletedTime = responseObject.getString(LABEL_DELETEDTIME);
			}
			if (responseObject.containsKey(LABEL_DELETEDBY)) {
				deletedBy = responseObject.getString(LABEL_DELETEDBY);
			}
			if (responseObject.containsKey(LABEL_DELETEDSTATUS)) {
				deletedStatus = responseObject.getString(LABEL_DELETEDSTATUS);
			}
			if (responseObject.containsKey(LABEL_EPPUSERNAME)) {
				eppUsername = responseObject.getString(LABEL_EPPUSERNAME);
			}
			if (responseObject.containsKey(LABEL_EPPPASSWORD)) {
				eppPassword = responseObject.getString(LABEL_EPPPASSWORD);
			}
			if (responseObject.containsKey(LABEL_EPPSTATUS)) {
				eppStatus = responseObject.getString(LABEL_EPPSTATUS);
			}
			if (responseObject.containsKey(LABEL_VIRTUAL_ACCOUNT_NUMBER)) {
				virtualAccountNumber = responseObject.getString(LABEL_VIRTUAL_ACCOUNT_NUMBER);
			}

			if (responseObject.containsKey(LABEL_RESPONSE_CODE)) {
				responseCode = responseObject.getString(LABEL_RESPONSE_CODE);
			}
			if (responseObject.containsKey(LABEL_RESPONSE_DESCRIPTION)) {
				responseDescription = responseObject.getString(LABEL_RESPONSE_DESCRIPTION);
			}
			if (responseObject.containsKey(LABEL_RESPONSE_TRANSACTION_ID)) {
				responseTransactionId = responseObject.getString(LABEL_RESPONSE_TRANSACTION_ID);
			}
			if (responseObject.containsKey(LABEL_EPP_NEWPASSWORD)) {
				eppNewPassword = responseObject.getString(LABEL_EPP_NEWPASSWORD);
			}
			if (responseObject.containsKey(LABEL_EPP_SESSION_TOKEN)) {
				eppSessionToken = responseObject.getString(LABEL_EPP_SESSION_TOKEN);
			}
			if (responseObject.containsKey(LABEL_REASON)) {
				reason = responseObject.getString(LABEL_REASON);
			}
			if (responseObject.containsKey(LABEL_TAX_PERCENTAGE)) {
				taxPercentage = responseObject.getString(LABEL_TAX_PERCENTAGE);
			}
			if (responseObject.containsKey(LABEL_CURRENCY_NAME)) {
				currencyName = responseObject.getString(LABEL_CURRENCY_NAME);
			}
			if (responseObject.containsKey(LABEL_PRICING_LEVEL)) {
				pricingLevelId = responseObject.getString(LABEL_PRICING_LEVEL);
			}
			if (responseObject.containsKey(LABEL_SERVER_CERTIFICATE_HASH)) {
				serverCertificateHash = responseObject.getString(LABEL_SERVER_CERTIFICATE_HASH);
			}
			if (responseObject.containsKey(LABEL_IP_ADDRESS)) {
				ipAddress = responseObject.getString(LABEL_IP_ADDRESS);
			}
			if (responseObject.containsKey(LABEL_APIUSERNAME)) {
				apiUsername = responseObject.getString(LABEL_APIUSERNAME);
			}
			if (responseObject.containsKey(LABEL_APIPASSWORD)) {
				apiPassword = responseObject.getString(LABEL_APIPASSWORD);
			}
			if (responseObject.containsKey(LABEL_APISTATUS)) {
				apiStatus = responseObject.getString(LABEL_APISTATUS);
			}
			if (responseObject.containsKey(LABEL_API_SESSION_TOKEN)) {
				apiSessionToken = responseObject.getString(LABEL_API_SESSION_TOKEN);
			}
			if (responseObject.containsKey(LABEL_ACTION_TYPE)) {
				actionType = responseObject.getString(LABEL_ACTION_TYPE);
			}
		}
	}

	
	
	

	public String getServerCertificateHash() {
		return serverCertificateHash;
	}

	public void setServerCertificateHash(String serverCertificateHash) {
		this.serverCertificateHash = serverCertificateHash;
	}

	public String getPricingLevelId() {
		return pricingLevelId;
	}

	public void setPricingLevelId(String pricingLevelId) {
		this.pricingLevelId = pricingLevelId;
	}

	public String getCurrencyName() {
		return currencyName;
	}

	public void setCurrencyName(String currencyName) {
		this.currencyName = currencyName;
	}

	public String getTaxPercentage() {
		return taxPercentage;
	}

	public void setTaxPercentage(String taxPercentage) {
		this.taxPercentage = taxPercentage;
	}

	public String getReason() {
		return reason;
	}

	public void setReason(String reason) {
		this.reason = reason;
	}

	public String getEppNewPassword() {
		return eppNewPassword;
	}

	public void setEppNewPassword(String eppNewPassword) {
		this.eppNewPassword = eppNewPassword;
	}

	public String getRegistrarId() {
		return registrarId;
	}

	public void setRegistrarId(String value) {
		registrarId = value;
	}
	
	
	
	// PK GETTER SETTER END

	public String getEmergencyCreditLimit() {
		return emergencyCreditLimit;
	}

	public void setEmergencyCreditLimit(String emergencyCreditLimit) {
		this.emergencyCreditLimit = emergencyCreditLimit;
	}

	public String getRegistrarName() {
		return registrarName;
	}

	public void setRegistrarName(String registrarName) {
		this.registrarName = registrarName;
	}

	public String getEppSessionToken() {
		return eppSessionToken;
	}

	public void setEppSessionToken(String eppSessionToken) {
		this.eppSessionToken = eppSessionToken;
	}

	public String getRegistrarUniqueCode() {
		return registrarUniqueCode;
	}

	public void setRegistrarUniqueCode(String value) {
		registrarUniqueCode = value;
	}

	public String getRegistrarGuid() {
		return registrarGuid;
	}

	public void setRegistrarGuid(String value) {
		registrarGuid = value;
	}

	public String getOrganizationName() {
		return organizationName;
	}

	public void setOrganizationName(String value) {
		organizationName = value;
	}

	public String getOrganizationAddress() {
		return organizationAddress;
	}

	public void setOrganizationAddress(String value) {
		organizationAddress = value;
	}
	
	public String getAddress1() {
		return address1;
	}

	public void setAddress1(String value) {
		address1 = value;
	}

	public String getAddress2() {
		return address2;
	}

	public void setAddress2(String value) {
		address2 = value;
	}

	public String getAddress3() {
		return address3;
	}

	public void setAddress3(String value) {
		address3 = value;
	}

	public String getPostalCode() {
		return postalCode;
	}

	public void setPostalCode(String value) {
		postalCode = value;
	}

	public String getCity() {
		return city;
	}

	public void setCity(String value) {
		city = value;
	}

	public String getProvince() {
		return province;
	}

	public void setProvince(String value) {
		province = value;
	}

	public String getCountry() {
		return country;
	}

	public void setCountry(String value) {
		country = value;
	}
	
	public String getTelephone() {
		return telephone;
	}

	public void setTelephone(String value) {
		telephone = value;
	}

	public String getFaximile() {
		return faximile;
	}

	public void setFaximile(String value) {
		faximile = value;
	}

	public String getEmail() {
		return email;
	}

	public void setEmail(String value) {
		email = value;
	}

	public String getRegistrarStatus() {
		return registrarStatus;
	}

	public void setRegistrarStatus(String value) {
		registrarStatus = value;
	}

	public String getRegistrarRatingReview() {
		return registrarRatingReview;
	}

	public void setRegistrarRatingReview(String value) {
		registrarRatingReview = value;
	}

	public String getRegistrationDate() {
		return registrationDate;
	}

	public void setRegistrationDate(String value) {
		registrationDate = value;
	}

	public String getExpireDate() {
		return expireDate;
	}

	public void setExpireDate(String value) {
		expireDate = value;
	}

	public String getRenewalDate() {
		return renewalDate;
	}

	public void setRenewalDate(String value) {
		renewalDate = value;
	}

	public String getDirectorName() {
		return directorName;
	}

	public void setDirectorName(String value) {
		directorName = value;
	}

	public String getNpwp() {
		return npwp;
	}

	public void setNpwp(String value) {
		npwp = value;
	}

	public String getSiup() {
		return siup;
	}

	public void setSiup(String value) {
		siup = value;
	}

	public String getSkPendirian() {
		return skPendirian;
	}

	public void setSkPendirian(String value) {
		skPendirian = value;
	}

	public String getCreatedTime() {
		return createdTime;
	}

	public void setCreatedTime(String value) {
		createdTime = value;
	}

	public String getCreatedBy() {
		return createdBy;
	}

	public void setCreatedBy(String value) {
		createdBy = value;
	}

	public String getModifiedTime() {
		return modifiedTime;
	}

	public void setModifiedTime(String value) {
		modifiedTime = value;
	}

	public String getModifiedBy() {
		return modifiedBy;
	}

	public void setModifiedBy(String value) {
		modifiedBy = value;
	}

	public String getDeletedTime() {
		return deletedTime;
	}

	public void setDeletedTime(String value) {
		deletedTime = value;
	}

	public String getDeletedBy() {
		return deletedBy;
	}

	public void setDeletedBy(String value) {
		deletedBy = value;
	}

	public String getDeletedStatus() {
		return deletedStatus;
	}

	public void setDeletedStatus(String value) {
		deletedStatus = value;
	}

	public String getEppUsername() {
		return eppUsername;
	}

	public void setEppUsername(String value) {
		eppUsername = value;
	}

	public String getEppPassword() {
		return eppPassword;
	}

	public void setEppPassword(String value) {
		eppPassword = value;
	}

	public String getEppStatus() {
		return eppStatus;
	}

	public void setEppStatus(String value) {
		eppStatus = value;
	}

	public String getEmailOperation() {
		return emailOperation;
	}

	public void setEmailOperation(String emailOperation) {
		this.emailOperation = emailOperation;
	}

	public String getEmailSecurity() {
		return emailSecurity;
	}

	public void setEmailSecurity(String emailSecurity) {
		this.emailSecurity = emailSecurity;
	}

	public String getEmailBilling() {
		return emailBilling;
	}

	public void setEmailBilling(String emailBilling) {
		this.emailBilling = emailBilling;
	}

	public String getEmailLegal() {
		return emailLegal;
	}

	public void setEmailLegal(String emailLegal) {
		this.emailLegal = emailLegal;
	}

	public String getEmailTransfer() {
		return emailTransfer;
	}

	public void setEmailTransfer(String emailTransfer) {
		this.emailTransfer = emailTransfer;
	}

	public String getEmailCreditLimit() {
		return emailCreditLimit;
	}

	public void setEmailCreditLimit(String emailCreditLimit) {
		this.emailCreditLimit = emailCreditLimit;
	}

	public String getEmailDeletion() {
		return emailDeletion;
	}

	public void setEmailDeletion(String emailDeletion) {
		this.emailDeletion = emailDeletion;
	}

	public String getEmailMarketing() {
		return emailMarketing;
	}

	public void setEmailMarketing(String emailMarketing) {
		this.emailMarketing = emailMarketing;
	}

	public String getWebsite() {
		return website;
	}

	public void setWebsite(String website) {
		this.website = website;
	}

	public String getLevel() {
		return level;
	}

	public void setLevel(String level) {
		this.level = level;
	}

	public String getManager() {
		return manager;
	}

	public void setManager(String manager) {
		this.manager = manager;
	}

	public String getCurrency() {
		return currency;
	}

	public void setCurrency(String currency) {
		this.currency = currency;
	}

	public String getCreditLimit() {
		return creditLimit;
	}

	public void setCreditLimit(String creditLimit) {
		this.creditLimit = creditLimit;
	}

	public String getRenewType() {
		return renewType;
	}

	public void setRenewType(String renewType) {
		this.renewType = renewType;
	}

	public String getBillingModel() {
		return billingModel;
	}

	public void setBillingModel(String billingModel) {
		this.billingModel = billingModel;
	}

	public String getBillingInterval() {
		return billingInterval;
	}

	public void setBillingInterval(String billingInterval) {
		this.billingInterval = billingInterval;
	}
	
	
	// foreign affairs

	// foreign affairs end


	public String getVirtualAccountNumber() {
		return virtualAccountNumber;
	}

	public void setVirtualAccountNumber(String virtualAccountNumber) {
		this.virtualAccountNumber = virtualAccountNumber;
	}
	
	public String getIpAddress() {
		return ipAddress;
	}

	public void setIpAddress(String ipAddress) {
		this.ipAddress = ipAddress;
	}
	
	public String getApiUsername() {
		return apiUsername;
	}

	public void setApiUsername(String apiUsername) {
		this.apiUsername = apiUsername;
	}

	public String getApiPassword() {
		return apiPassword;
	}

	public void setApiPassword(String apiPassword) {
		this.apiPassword = apiPassword;
	}

	public String getApiSessionToken() {
		return apiSessionToken;
	}

	public void setApiSessionToken(String apiSessionToken) {
		this.apiSessionToken = apiSessionToken;
	}
	
	public String getApiStatus() {
		return apiStatus;
	}

	public void setApiStatus(String apiStatus) {
		this.apiStatus = apiStatus;
	}
	
	public String getActionType() {
		return actionType;
	}

	public void setActionType(String actionType) {
		this.actionType = actionType;
	}

	public String getResponseCode() {
		return responseCode;
	}

	public void setResponseCode(String responseCode) {
		this.responseCode = responseCode;
	}

	public String getResponseDescription() {
		return responseDescription;
	}

	public void setResponseDescription(String responseDescription) {
		this.responseDescription = responseDescription;
	}

	public String getResponseTransactionId() {
		return responseTransactionId;
	}

	public void setResponseTransactionId(String responseTransactionId) {
		this.responseTransactionId = responseTransactionId;
	}

	public List<String> getDomainList() {
		return domainList;
	}

	public void setDomainList(List<String> domainList) {
		this.domainList = domainList;
	}

	public String toJsonString() {
		String result = "";

		try {
			JsonObjectBuilder jsonBuilder = Json.createObjectBuilder();

			if (jsonBuilder != null) {
				if (registrarId != null) {
					jsonBuilder.add(LABEL_REGISTRARID, registrarId);
				}
				if (registrarName != null) {
					jsonBuilder.add(LABEL_REGISTRAR_NAME, registrarName);
				}
				if (eppSessionToken != null) {
					jsonBuilder.add(LABEL_EPP_SESSION_TOKEN, eppSessionToken);
				}
				if (registrarUniqueCode != null) {
					jsonBuilder.add(LABEL_REGISTRARUNIQUECODE, registrarUniqueCode);
				}
				if (registrarGuid != null) {
					jsonBuilder.add(LABEL_REGISTRARGUID, registrarGuid);
				}
				if (organizationName != null) {
					jsonBuilder.add(LABEL_ORGANIZATIONNAME, organizationName);
				}
				if (organizationAddress != null) {
					jsonBuilder.add(LABEL_ORGANIZATIONADDRESS, organizationAddress);
				}
				if (address1 != null) {
					jsonBuilder.add(LABEL_ADDRESS1, address1);
				}
				if (address2 != null) {
					jsonBuilder.add(LABEL_ADDRESS2, address2);
				}
				if (address3 != null) {
					jsonBuilder.add(LABEL_ADDRESS3, address3);
				}
				if (postalCode != null) {
					jsonBuilder.add(LABEL_POSTALCODE, postalCode);
				}
				if (city != null) {
					jsonBuilder.add(LABEL_CITY, city);
				}
				if (province != null) {
					jsonBuilder.add(LABEL_PROVINCE, province);
				}
				if (country != null) {
					jsonBuilder.add(LABEL_COUNTRY, country);
				}
				if (telephone != null) {
					jsonBuilder.add(LABEL_TELEPHONE, telephone);
				}
				if (faximile != null) {
					jsonBuilder.add(LABEL_FAXIMILE, faximile);
				}
				if (email != null) {
					jsonBuilder.add(LABEL_EMAIL, email);
				}
				if (emailOperation != null) {
					jsonBuilder.add(LABEL_EMAILOPERATION, emailOperation);
				}
				if (emailSecurity != null) {
					jsonBuilder.add(LABEL_EMAILSECURITY, emailSecurity);
				}
				if (emailBilling != null) {
					jsonBuilder.add(LABEL_EMAILBILLING, emailBilling);
				}
				if (emailLegal != null) {
					jsonBuilder.add(LABEL_EMAILLEGAL, emailLegal);
				}
				if (emailTransfer != null) {
					jsonBuilder.add(LABEL_EMAILTRANSFER, emailTransfer);
				}
				if (emailCreditLimit != null) {
					jsonBuilder.add(LABEL_EMAILCREDITLIMIT, emailCreditLimit);
				}
				if (emailDeletion != null) {
					jsonBuilder.add(LABEL_EMAILDELETION, emailDeletion);
				}
				if (emailMarketing != null) {
					jsonBuilder.add(LABEL_EMAILMARKETING, emailMarketing);
				}
				if (website != null) {
					jsonBuilder.add(LABEL_WEBSITE, website);
				}
				if (level != null) {
					jsonBuilder.add(LABEL_LEVEL, level);
				}
				if (manager != null) {
					jsonBuilder.add(LABEL_MANAGER, manager);
				}
				if (currency != null) {
					jsonBuilder.add(LABEL_CURRENCY, currency);
				}
				if (creditLimit != null) {
					jsonBuilder.add(LABEL_CREDITLIMIT, creditLimit);
				}
				if (emergencyCreditLimit != null) {
					jsonBuilder.add(LABEL_EMERGENCY_CREDITLIMIT, emergencyCreditLimit);
				}
				if (renewType != null) {
					jsonBuilder.add(LABEL_RENEWTYPE, renewType);
				}
				if (billingModel != null) {
					jsonBuilder.add(LABEL_BILLINGMODEL, billingModel);
				}
				if (billingInterval != null) {
					jsonBuilder.add(LABEL_BILLINGINTERVAL, billingInterval);
				}
				if (registrarStatus != null) {
					jsonBuilder.add(LABEL_REGISTRARSTATUS, registrarStatus);
				}
				if (registrarRatingReview != null) {
					jsonBuilder.add(LABEL_REGISTRARRATINGREVIEW, registrarRatingReview);
				}
				if (registrationDate != null) {
					jsonBuilder.add(LABEL_REGISTRATIONDATE, registrationDate);
				}
				if (expireDate != null) {
					jsonBuilder.add(LABEL_EXPIREDATE, expireDate);
				}
				if (renewalDate != null) {
					jsonBuilder.add(LABEL_RENEWALDATE, renewalDate);
				}
				if (directorName != null) {
					jsonBuilder.add(LABEL_DIRECTORNAME, directorName);
				}
				if (npwp != null) {
					jsonBuilder.add(LABEL_NPWP, npwp);
				}
				if (siup != null) {
					jsonBuilder.add(LABEL_SIUP, siup);
				}
				if (skPendirian != null) {
					jsonBuilder.add(LABEL_SKPENDIRIAN, skPendirian);
				}
				if (createdTime != null) {
					jsonBuilder.add(LABEL_CREATEDTIME, createdTime);
				}
				if (createdBy != null) {
					jsonBuilder.add(LABEL_CREATEDBY, createdBy);
				}
				if (modifiedTime != null) {
					jsonBuilder.add(LABEL_MODIFIEDTIME, modifiedTime);
				}
				if (modifiedBy != null) {
					jsonBuilder.add(LABEL_MODIFIEDBY, modifiedBy);
				}
				if (deletedTime != null) {
					jsonBuilder.add(LABEL_DELETEDTIME, deletedTime);
				}
				if (deletedBy != null) {
					jsonBuilder.add(LABEL_DELETEDBY, deletedBy);
				}
				if (deletedStatus != null) {
					jsonBuilder.add(LABEL_DELETEDSTATUS, deletedStatus);
				}
				if (eppUsername != null) {
					jsonBuilder.add(LABEL_EPPUSERNAME, eppUsername);
				}
				if (eppPassword != null) {
					jsonBuilder.add(LABEL_EPPPASSWORD, eppPassword);
				}
				if (eppStatus != null) {
					jsonBuilder.add(LABEL_EPPSTATUS, eppStatus);
				}
				if (responseCode != null) {
					jsonBuilder.add(LABEL_RESPONSE_CODE, responseCode);
				}
				if (responseDescription != null) {
					jsonBuilder.add(LABEL_RESPONSE_DESCRIPTION, responseDescription);
				}
				if (responseTransactionId != null) {
					jsonBuilder.add(LABEL_RESPONSE_TRANSACTION_ID, responseTransactionId);
				}
				if (eppNewPassword != null) {
					jsonBuilder.add(LABEL_EPP_NEWPASSWORD, eppNewPassword);
				}
				if (reason != null) {
					jsonBuilder.add(LABEL_REASON, reason);
				}
				if (taxPercentage != null) {
					jsonBuilder.add(LABEL_TAX_PERCENTAGE, taxPercentage);
				}
				if (currencyName != null) {
					jsonBuilder.add(LABEL_CURRENCY_NAME, currencyName);
				}
				if (pricingLevelId != null) {
					jsonBuilder.add(LABEL_PRICING_LEVEL, pricingLevelId);
				}
				if (virtualAccountNumber != null) {
					jsonBuilder.add(LABEL_VIRTUAL_ACCOUNT_NUMBER, virtualAccountNumber);
				}
				if (serverCertificateHash != null) {
					jsonBuilder.add(LABEL_SERVER_CERTIFICATE_HASH, serverCertificateHash);
				}
				if (ipAddress != null) {
					jsonBuilder.add(LABEL_IP_ADDRESS, ipAddress);
				}
				if (apiSessionToken != null) {
					jsonBuilder.add(LABEL_API_SESSION_TOKEN, apiSessionToken);
				}
				if (apiUsername != null) {
					jsonBuilder.add(LABEL_APIUSERNAME, apiUsername);
				}
				if (apiPassword != null) {
					jsonBuilder.add(LABEL_APIPASSWORD, apiPassword);
				}
				if (apiStatus != null) {
					jsonBuilder.add(LABEL_APISTATUS, apiStatus);
				}
				if (actionType != null) {
					jsonBuilder.add(LABEL_ACTION_TYPE, actionType);
				}
				result = jsonBuilder.build().toString();
			}

		} catch (Exception e) {
			e.printStackTrace();
		}
		return result;
	}
}

