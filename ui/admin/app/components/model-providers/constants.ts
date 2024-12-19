export const CommonModelProviderIds = {
    OLLAMA: "ollama-model-provider",
    GROQ: "groq-model-provider",
    VOYAGE: "voyage-model-provider",
    ANTHROPIC: "anthropic-model-provider",
    OPENAI: "openai-model-provider",
    AZURE_OPENAI: "azure-openai-model-provider",
    ANTHROPIC_BEDROCK: "anthropic-bedrock-model-provider",
};

export const ModelProviderLinks = {
    [CommonModelProviderIds.VOYAGE]: "https://www.voyageai.com/",
    [CommonModelProviderIds.OLLAMA]: "https://ollama.com/",
    [CommonModelProviderIds.GROQ]: "https://groq.com/",
    [CommonModelProviderIds.AZURE_OPENAI]:
        "https://azure.microsoft.com/en-us/explore/",
    [CommonModelProviderIds.ANTHROPIC]: "https://www.anthropic.com",
    [CommonModelProviderIds.OPENAI]: "https://openai.com/",
    [CommonModelProviderIds.ANTHROPIC_BEDROCK]:
        "https://aws.amazon.com/bedrock/claude/",
};

export const ModelProviderConfigurationLinks = {
    [CommonModelProviderIds.AZURE_OPENAI]:
        "https://docs.otto8.ai/configuration/model-providers#azure-openai",
};

export const RecommendedModelProviders = [
    CommonModelProviderIds.OPENAI,
    CommonModelProviderIds.AZURE_OPENAI,
];

export const ModelProviderRequiredTooltips: {
    [key: string]: {
        [key: string]: string;
    };
} = {
    [CommonModelProviderIds.OLLAMA]: {
        Host: "IP Address for the ollama server (eg. 127.0.0.1:1234)",
    },
    [CommonModelProviderIds.GROQ]: {
        "Api Key":
            "Groq API Key. Can be created and fetched from https://console.groq.com/keys",
    },
    [CommonModelProviderIds.AZURE_OPENAI]: {
        Endpoint:
            "Endpoint for the Azure OpenAI service (e.g. https://<resource-name>.<region>.api.cognitive.microsoft.com/)",
        "Client Id":
            "Unique identifier for the application when using Azure Active Directory. Can typically be found in App Registrations > [application].",
        "Client Secret":
            "Password or key that app uses to authenticate with Azure Active Directory. Can typically be found in App Registrations > [application] > Certificates & Secrets",
        "Tenant Id":
            "Identifier of instance where the app and resources reside. Can typically be found in Azure Active Directory > Overview > Directory ID",
        "Subscription Id":
            "Identifier of user's Azure subscription. Can typically be found in Azure Portal > Subscriptions > Overview.",
        "Resource Group":
            "Container that holds related Azure resources. Can typically be found in Azure Portal > Resource Groups > [OpenAI Resource Group] > Overview",
    },
    [CommonModelProviderIds.ANTHROPIC_BEDROCK]: {
        "Access Key ID": "AWS Access Key ID",
        "Secret Access Key": "AWS Secret Access Key",
        "Session Token": "AWS Session Token",
        Region: "AWS Region - make sure that the models you want to use are available in this region: https://docs.aws.amazon.com/bedrock/latest/userguide/models-regions.html",
    },
};

export const ModelProviderSensitiveFields: Record<string, boolean | undefined> =
    {
        // OpenAI
        OBOT_OPENAI_MODEL_PROVIDER_API_KEY: true,

        // Azure OpenAI
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT: false,
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID: false,
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET: true,
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID: false,
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID: false,
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP: false,

        // Anthropic
        OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY: true,

        // Voyage
        OBOT_VOYAGE_MODEL_PROVIDER_API_KEY: true,

        // Ollama
        OBOT_OLLAMA_MODEL_PROVIDER_HOST: true,

        // Groq
        OBOT_GROQ_MODEL_PROVIDER_API_KEY: true,

        // Anthropic Bedrock
        OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID: true,
        OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY: true,
        OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_SESSION_TOKEN: true,
        OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_REGION: false,
    };
