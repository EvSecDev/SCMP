
export type PromptReq = {
    associatedDataID: string;
    promptID: string;
    title: string;
    details: string;
    type: string;
};

export type PromptAnswer = {
    associatedDataID: string;
    promptID: string;
    encodedData: string;
};

export type DeployStatus = {
    deploymentID: string;
    status: string;
    pending?: boolean;
    pendingAction?: PromptReq[];
    [key: string]: unknown;
};

export type DeployStart = {
    mode: string;
    type: string;
    options: {
        allowDeletions: boolean;
        runInstall: boolean;
        disableReloads: boolean;
        disableSudo: boolean;
        ignoreHostState: boolean;
        force: boolean;
        autoCommitRollbackEnabled: boolean;
        commitID: string;
        hostOverride: string;
        fileOverride: string;
        runAsUser: string;
        maxSSHConnections: number;
        maxSSHChannels: number;
        maxCommandRuntime: number;
        verbosity: number;
    };
};

export type DeployAbort = {
    deploymentID: string;
    stopRequested: boolean;
};

export type DeployOutput = {
    deploymentID: string;
    status: string;
    summary?: DeploymentSummary;
    rawOutput?: string;
    [key: string]: unknown;
};

export type DeploymentSummary = {
    Status: string;
    "Start-Time": string;
    "End-Time": string;
    "Elapsed-Time": string;
    "Transferred-Size": string;
    Counters: {
        Hosts: number;
        Items: number;
        "Hosts-Completed": number;
        "Items-Completed": number;
        "Hosts-Failed": number;
        "Items-Failed": number;
    };
    "Deployment-Commit-Hash": string;
    Hosts?: HostSummary[];
};

export type HostSummary = {
    Name: string;
    Status?: string;
    "Error-Message"?: string;
    "Total-Items"?: number;
    "Transferred-Size"?: string;
    Items?: ItemSummary[];
};

export type ItemSummary = {
    Name: string;
    "Deployment-Action": string;
    Status?: string;
    "Error-Message"?: string;
};
