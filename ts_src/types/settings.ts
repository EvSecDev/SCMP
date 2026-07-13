
export type VersionInfo = {
    fullProgramName: string;
    versionString: string;
    platform: string;
    architecture: string;
    apiBrowserLocation: string;
    docsLink: string;
    [key: string]: unknown;
};

export type RepoList = {
    repositories: string[];
    [key: string]: unknown;
};

export type HostSettings = {
    state: string;
    ignoresUniversal: boolean;
    requiresVault: boolean;
    groups: string[];
    proxy?: string;
    address: string;
    loginUser: string;
    identityFile?: string;
    connectTimeout?: number;
    [key: string]: unknown;
};

export type HostList = {
    hosts: string[];
    hostDetails?: Record<string, HostSettings>;
    [key: string]: unknown;
};
