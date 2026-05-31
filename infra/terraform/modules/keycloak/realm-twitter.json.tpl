{
  "realm": "twitter",
  "enabled": true,
  "sslRequired": "external",
  "registrationAllowed": true,
  "loginWithEmailAllowed": true,
  "duplicateEmailsAllowed": false,
  "resetPasswordAllowed": true,
  "editUsernameAllowed": true,
  "accessTokenLifespan": 900,
  "ssoSessionMaxLifespan": 36000,
  "components": {
    "org.keycloak.keys.KeyProvider": [
      {
        "name": "rsa-stage-key",
        "providerId": "rsa",
        "subComponents": {},
        "config": {
          "privateKey": ["${private_key_b64}"],
          "certificate": ["${certificate_b64}"],
          "priority": ["100"],
          "enabled": ["true"],
          "active": ["true"],
          "algorithm": ["RS256"]
        }
      }
    ]
  },
  "clients": [
    {
      "clientId": "twitter-app",
      "name": "Twitter Clone Frontend",
      "enabled": true,
      "publicClient": true,
      "standardFlowEnabled": true,
      "directAccessGrantsEnabled": true,
      "redirectUris": ["https://${domain}/*"],
      "webOrigins": ["https://${domain}"]
    },
    {
      "clientId": "kong-admin",
      "name": "User-service admin client",
      "enabled": true,
      "publicClient": false,
      "secret": "$${env.KC_ADMIN_CLIENT_SECRET}",
      "standardFlowEnabled": false,
      "directAccessGrantsEnabled": false,
      "serviceAccountsEnabled": true,
      "attributes": {
        "use.refresh.tokens": "false"
      }
    }
  ],
  "identityProviders": [
    {
      "alias": "google",
      "displayName": "Google",
      "providerId": "google",
      "enabled": true,
      "trustEmail": true,
      "storeToken": false,
      "addReadTokenRoleOnCreate": false,
      "authenticateByDefault": false,
      "linkOnly": false,
      "firstBrokerLoginFlowAlias": "first broker login",
      "config": {
        "clientId": "$${env.KC_GOOGLE_CLIENT_ID}",
        "clientSecret": "$${env.KC_GOOGLE_CLIENT_SECRET}",
        "syncMode": "IMPORT",
        "useJwksUrl": "true",
        "defaultScope": "openid email profile"
      }
    }
  ],
  "authenticationFlows": [
    {
      "alias": "first broker login",
      "description": "Actions taken after first broker login with Identity Provider account, which is not yet linked to any Keycloak account",
      "providerId": "basic-flow",
      "topLevel": true,
      "builtIn": false,
      "authenticationExecutions": [
        {
          "authenticator": "idp-review-profile",
          "authenticatorConfig": "review profile config",
          "requirement": "DISABLED",
          "priority": 10,
          "autheticatorFlow": false,
          "userSetupAllowed": false
        },
        {
          "flowAlias": "User creation or linking",
          "requirement": "REQUIRED",
          "priority": 20,
          "autheticatorFlow": true,
          "userSetupAllowed": false
        }
      ]
    },
    {
      "alias": "User creation or linking",
      "description": "Flow for the existing/non-existing user alternatives in first broker login",
      "providerId": "basic-flow",
      "topLevel": false,
      "builtIn": false,
      "authenticationExecutions": [
        {
          "authenticator": "idp-create-user-if-unique",
          "authenticatorConfig": "create unique user config",
          "requirement": "ALTERNATIVE",
          "priority": 10,
          "autheticatorFlow": false,
          "userSetupAllowed": false
        },
        {
          "flowAlias": "Handle Existing Account",
          "requirement": "ALTERNATIVE",
          "priority": 20,
          "autheticatorFlow": true,
          "userSetupAllowed": false
        }
      ]
    },
    {
      "alias": "Handle Existing Account",
      "description": "Handle what to do if there is existing account with same email/username as authenticated identity provider user",
      "providerId": "basic-flow",
      "topLevel": false,
      "builtIn": false,
      "authenticationExecutions": [
        {
          "authenticator": "idp-confirm-link",
          "requirement": "REQUIRED",
          "priority": 10,
          "autheticatorFlow": false,
          "userSetupAllowed": false
        },
        {
          "flowAlias": "Account verification options",
          "requirement": "REQUIRED",
          "priority": 20,
          "autheticatorFlow": true,
          "userSetupAllowed": false
        }
      ]
    },
    {
      "alias": "Account verification options",
      "description": "Method with which to verify the existing account",
      "providerId": "basic-flow",
      "topLevel": false,
      "builtIn": false,
      "authenticationExecutions": [
        {
          "authenticator": "idp-email-verification",
          "requirement": "ALTERNATIVE",
          "priority": 10,
          "autheticatorFlow": false,
          "userSetupAllowed": false
        },
        {
          "flowAlias": "Verify Existing Account by Re-authentication",
          "requirement": "ALTERNATIVE",
          "priority": 20,
          "autheticatorFlow": true,
          "userSetupAllowed": false
        }
      ]
    },
    {
      "alias": "Verify Existing Account by Re-authentication",
      "description": "Reauthentication of existing account",
      "providerId": "basic-flow",
      "topLevel": false,
      "builtIn": false,
      "authenticationExecutions": [
        {
          "authenticator": "idp-username-password-form",
          "requirement": "REQUIRED",
          "priority": 10,
          "autheticatorFlow": false,
          "userSetupAllowed": false
        },
        {
          "authenticator": "auth-otp-form",
          "requirement": "OPTIONAL",
          "priority": 20,
          "autheticatorFlow": false,
          "userSetupAllowed": false
        }
      ]
    }
  ],
  "authenticatorConfig": [
    {
      "alias": "review profile config",
      "config": {
        "update.profile.on.first.login": "off"
      }
    },
    {
      "alias": "create unique user config",
      "config": {
        "require.password.update.after.registration": "false"
      }
    }
  ],
  "requiredActions": [
    {
      "alias": "PROVISION_USER",
      "name": "Provision user in user-service",
      "providerId": "PROVISION_USER",
      "enabled": true,
      "defaultAction": false,
      "priority": 0,
      "config": {}
    }
  ],
  "roles": {
    "realm": [],
    "client": {}
  },
  "users": [
    {
      "username": "service-account-kong-admin",
      "enabled": true,
      "serviceAccountClientId": "kong-admin",
      "clientRoles": {
        "realm-management": ["manage-users", "view-users", "query-users"]
      }
    }
  ]
}
