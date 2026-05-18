package com.twitter.keycloak;

import jakarta.ws.rs.core.Response;
import org.keycloak.authentication.RequiredActionContext;
import org.keycloak.authentication.RequiredActionProvider;
import org.keycloak.models.UserModel;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;

/**
 * Keycloak Required Action that provisions a user row in user-service
 * synchronously during the first login, before any token is issued.
 *
 * Flow:
 *   1. User authenticates via Google (or any IDP).
 *   2. Keycloak evaluates required actions — this one fires if "PROVISION_USER"
 *      is set on the account (added by the first-broker-login flow).
 *   3. POST /internal/provision is called with the Keycloak sub, email,
 *      and display name.
 *   4. On success, the action is removed and context.success() is called
 *      so Keycloak proceeds to issue the token.
 */
public class ProvisionUserAction implements RequiredActionProvider {

    static final String PROVIDER_ID = "PROVISION_USER";

    private static final HttpClient HTTP = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(5))
            .build();

    @Override
    public void evaluateTriggers(RequiredActionContext context) {
        UserModel user = context.getUser();
        // Fire only once — after provisioning we remove the action.
        if (user.getFirstAttribute("provisioned") == null) {
            user.addRequiredAction(PROVIDER_ID);
        }
    }

    @Override
    public void requiredActionChallenge(RequiredActionContext context) {
        // No UI challenge needed — provision inline and succeed immediately.
        processAction(context);
    }

    @Override
    public void processAction(RequiredActionContext context) {
        UserModel user = context.getUser();
        String sub = user.getId();
        String email = user.getEmail() != null ? user.getEmail() : "";
        String displayName = user.getFirstName() != null ? user.getFirstName() : "";
        if (user.getLastName() != null && !user.getLastName().isEmpty()) {
            displayName = displayName.isEmpty()
                    ? user.getLastName()
                    : displayName + " " + user.getLastName();
        }

        String userServiceURL = System.getenv("USER_SERVICE_URL");
        String serviceToken  = System.getenv("SERVICE_TOKEN");
        if (userServiceURL == null || userServiceURL.isEmpty()) {
            userServiceURL = "http://user-service:8080";
        }

        String body = String.format(
                "{\"keycloak_sub\":\"%s\",\"email\":\"%s\",\"display_name\":\"%s\"}",
                jsonEscape(sub), jsonEscape(email), jsonEscape(displayName));

        try {
            HttpRequest request = HttpRequest.newBuilder()
                    .uri(URI.create(userServiceURL + "/internal/provision"))
                    .timeout(Duration.ofSeconds(5))
                    .header("Content-Type", "application/json")
                    .header("Authorization", "Bearer " + (serviceToken != null ? serviceToken : ""))
                    .POST(HttpRequest.BodyPublishers.ofString(body))
                    .build();

            HttpResponse<String> response = HTTP.send(request, HttpResponse.BodyHandlers.ofString());
            int status = response.statusCode();

            // 201 Created or 200 OK (idempotent re-provision) — both are success.
            if (status == 201 || status == 200) {
                user.setSingleAttribute("provisioned", "true");
                user.removeRequiredAction(PROVIDER_ID);
                context.success();
            } else {
                // Provisioning failed — block token issuance and show an error page.
                context.challenge(
                        Response.status(Response.Status.INTERNAL_SERVER_ERROR)
                                .entity("Account setup failed. Please try again.")
                                .type("text/plain")
                                .build());
            }
        } catch (Exception e) {
            context.challenge(
                    Response.status(Response.Status.INTERNAL_SERVER_ERROR)
                            .entity("Account setup error: " + e.getMessage())
                            .type("text/plain")
                            .build());
        }
    }

    @Override
    public void close() {}

    private static String jsonEscape(String s) {
        if (s == null) return "";
        return s.replace("\\", "\\\\")
                .replace("\"", "\\\"")
                .replace("\n", "\\n")
                .replace("\r", "\\r")
                .replace("\t", "\\t");
    }
}
