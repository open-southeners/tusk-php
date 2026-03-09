package container

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

type ServiceBinding struct {
	Abstract  string
	Concrete  string
	Singleton bool
	Source    string
	Tags     []string
	Alias    string
}

type ContainerAnalyzer struct {
	mu        sync.RWMutex
	bindings  map[string]*ServiceBinding
	aliases   map[string]string
	tags      map[string][]string
	index     *symbols.Index
	rootPath  string
	framework string
}

func NewContainerAnalyzer(index *symbols.Index, rootPath, framework string) *ContainerAnalyzer {
	return &ContainerAnalyzer{
		bindings:  make(map[string]*ServiceBinding),
		aliases:   make(map[string]string),
		tags:      make(map[string][]string),
		index:     index,
		rootPath:  rootPath,
		framework: framework,
	}
}

func (ca *ContainerAnalyzer) Analyze() {
	switch ca.framework {
	case "laravel":
		ca.analyzeLaravel()
	case "symfony":
		ca.analyzeSymfony()
	}
}

func (ca *ContainerAnalyzer) ResolveDependency(typeName string) *ServiceBinding {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	if abstract, ok := ca.aliases[typeName]; ok {
		if binding, ok := ca.bindings[abstract]; ok {
			return binding
		}
	}
	if binding, ok := ca.bindings[typeName]; ok {
		return binding
	}
	return nil
}

func (ca *ContainerAnalyzer) GetBindings() map[string]*ServiceBinding {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	result := make(map[string]*ServiceBinding, len(ca.bindings))
	for k, v := range ca.bindings {
		result[k] = v
	}
	return result
}

type InjectedDependency struct {
	ParamName        string
	TypeHint         string
	ResolvedConcrete string
	IsSingleton      bool
}

func (ca *ContainerAnalyzer) AnalyzeConstructorInjection(classFQN string) []InjectedDependency {
	sym := ca.index.Lookup(classFQN)
	if sym == nil {
		return nil
	}
	var deps []InjectedDependency
	for _, child := range sym.Children {
		if child.Kind == symbols.KindMethod && child.Name == "__construct" {
			for _, param := range child.Params {
				dep := InjectedDependency{ParamName: param.Name, TypeHint: param.Type}
				if binding := ca.ResolveDependency(param.Type); binding != nil {
					dep.ResolvedConcrete = binding.Concrete
					dep.IsSingleton = binding.Singleton
				}
				deps = append(deps, dep)
			}
		}
	}
	return deps
}

var (
	laravelBindRegex      = regexp.MustCompile(`\$this->app->bind\(\s*([^,]+),\s*([^)]+)\)`)
	laravelSingletonRegex = regexp.MustCompile(`\$this->app->singleton\(\s*([^,]+),\s*([^)]+)\)`)
)

func (ca *ContainerAnalyzer) analyzeLaravel() {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.registerLaravelDefaults()
	providersDir := filepath.Join(ca.rootPath, "app", "Providers")
	ca.scanDirectory(providersDir, ca.parseLaravelProvider)
}

func (ca *ContainerAnalyzer) registerLaravelDefaults() {
	defaults := []ServiceBinding{
		{Abstract: "Illuminate\\Contracts\\Auth\\Factory", Concrete: "Illuminate\\Auth\\AuthManager", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Cache\\Factory", Concrete: "Illuminate\\Cache\\CacheManager", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Config\\Repository", Concrete: "Illuminate\\Config\\Repository", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Container\\Container", Concrete: "Illuminate\\Container\\Container", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Events\\Dispatcher", Concrete: "Illuminate\\Events\\Dispatcher", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Filesystem\\Factory", Concrete: "Illuminate\\Filesystem\\FilesystemManager", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Http\\Kernel", Concrete: "App\\Http\\Kernel", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Mail\\Mailer", Concrete: "Illuminate\\Mail\\Mailer", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Queue\\Factory", Concrete: "Illuminate\\Queue\\QueueManager", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Routing\\ResponseFactory", Concrete: "Illuminate\\Routing\\ResponseFactory", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Routing\\UrlGenerator", Concrete: "Illuminate\\Routing\\UrlGenerator", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Session\\Session", Concrete: "Illuminate\\Session\\Store", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\Validation\\Factory", Concrete: "Illuminate\\Validation\\Factory", Singleton: true},
		{Abstract: "Illuminate\\Contracts\\View\\Factory", Concrete: "Illuminate\\View\\Factory", Singleton: true},
		{Abstract: "auth", Concrete: "Illuminate\\Auth\\AuthManager", Singleton: true},
		{Abstract: "cache", Concrete: "Illuminate\\Cache\\CacheManager", Singleton: true},
		{Abstract: "config", Concrete: "Illuminate\\Config\\Repository", Singleton: true},
		{Abstract: "db", Concrete: "Illuminate\\Database\\DatabaseManager", Singleton: true},
		{Abstract: "events", Concrete: "Illuminate\\Events\\Dispatcher", Singleton: true},
		{Abstract: "log", Concrete: "Illuminate\\Log\\LogManager", Singleton: true},
		{Abstract: "queue", Concrete: "Illuminate\\Queue\\QueueManager", Singleton: true},
		{Abstract: "request", Concrete: "Illuminate\\Http\\Request", Singleton: true},
		{Abstract: "router", Concrete: "Illuminate\\Routing\\Router", Singleton: true},
		{Abstract: "session", Concrete: "Illuminate\\Session\\SessionManager", Singleton: true},
		{Abstract: "view", Concrete: "Illuminate\\View\\Factory", Singleton: true},
	}
	for _, b := range defaults {
		binding := b
		ca.bindings[binding.Abstract] = &binding
	}
}

func (ca *ContainerAnalyzer) parseLaravelProvider(path string, content string) {
	for _, match := range laravelBindRegex.FindAllStringSubmatch(content, -1) {
		abstract := cleanPHPString(match[1])
		concrete := cleanPHPString(match[2])
		ca.bindings[abstract] = &ServiceBinding{Abstract: abstract, Concrete: concrete, Source: path}
	}
	for _, match := range laravelSingletonRegex.FindAllStringSubmatch(content, -1) {
		abstract := cleanPHPString(match[1])
		concrete := cleanPHPString(match[2])
		ca.bindings[abstract] = &ServiceBinding{Abstract: abstract, Concrete: concrete, Singleton: true, Source: path}
	}
}

func (ca *ContainerAnalyzer) analyzeSymfony() {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.registerSymfonyDefaults()
	ca.parseSymfonyServicesYAML()
	ca.scanDirectory(filepath.Join(ca.rootPath, "src"), ca.parseSymfonyAttributes)
}

func (ca *ContainerAnalyzer) registerSymfonyDefaults() {
	defaults := []ServiceBinding{
		{Abstract: "Symfony\\Component\\HttpFoundation\\RequestStack", Concrete: "Symfony\\Component\\HttpFoundation\\RequestStack", Singleton: true},
		{Abstract: "Symfony\\Component\\HttpKernel\\KernelInterface", Concrete: "App\\Kernel", Singleton: true},
		{Abstract: "Symfony\\Component\\Routing\\RouterInterface", Concrete: "Symfony\\Component\\Routing\\Router", Singleton: true},
		{Abstract: "Symfony\\Component\\EventDispatcher\\EventDispatcherInterface", Concrete: "Symfony\\Component\\EventDispatcher\\EventDispatcher", Singleton: true},
		{Abstract: "Psr\\Log\\LoggerInterface", Concrete: "Symfony\\Bridge\\Monolog\\Logger", Singleton: true},
		{Abstract: "Doctrine\\ORM\\EntityManagerInterface", Concrete: "Doctrine\\ORM\\EntityManager", Singleton: true},
		{Abstract: "Symfony\\Component\\Security\\Core\\Authorization\\AuthorizationCheckerInterface", Concrete: "Symfony\\Component\\Security\\Core\\Authorization\\AuthorizationChecker", Singleton: true},
		{Abstract: "Symfony\\Component\\Serializer\\SerializerInterface", Concrete: "Symfony\\Component\\Serializer\\Serializer", Singleton: true},
		{Abstract: "Symfony\\Component\\Validator\\Validator\\ValidatorInterface", Concrete: "Symfony\\Component\\Validator\\Validator\\RecursiveValidator", Singleton: true},
		{Abstract: "Symfony\\Component\\Mailer\\MailerInterface", Concrete: "Symfony\\Component\\Mailer\\Mailer", Singleton: true},
		{Abstract: "Symfony\\Component\\Messenger\\MessageBusInterface", Concrete: "Symfony\\Component\\Messenger\\MessageBus", Singleton: true},
		{Abstract: "Twig\\Environment", Concrete: "Twig\\Environment", Singleton: true},
	}
	for _, b := range defaults {
		binding := b
		ca.bindings[binding.Abstract] = &binding
	}
}

func (ca *ContainerAnalyzer) parseSymfonyServicesYAML() {
	for _, name := range []string{"services.yaml", "services.yml"} {
		yamlPath := filepath.Join(ca.rootPath, "config", name)
		content, err := os.ReadFile(yamlPath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")
		var currentService string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "#") {
				svcName := strings.TrimSuffix(trimmed, ":")
				if strings.Contains(svcName, "\\") || strings.HasPrefix(svcName, "App\\") {
					currentService = svcName
					ca.bindings[currentService] = &ServiceBinding{Abstract: currentService, Concrete: currentService, Singleton: true, Source: yamlPath}
				}
			}
			if currentService != "" && strings.Contains(trimmed, "class:") {
				parts := strings.SplitN(trimmed, ":", 2)
				if len(parts) == 2 {
					if b, ok := ca.bindings[currentService]; ok {
						b.Concrete = strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}
}

func (ca *ContainerAnalyzer) parseSymfonyAttributes(path string, content string) {
	if !strings.HasSuffix(path, ".php") {
		return
	}
	file := parser.ParseFile(content)
	if file == nil {
		return
	}
	ns := file.Namespace
	for _, cls := range file.Classes {
		fqn := ns + "\\" + cls.Name
		if strings.Contains(path, "src"+string(filepath.Separator)) {
			if _, exists := ca.bindings[fqn]; !exists {
				ca.bindings[fqn] = &ServiceBinding{Abstract: fqn, Concrete: fqn, Singleton: true, Source: path}
			}
		}
		for _, iface := range cls.Implements {
			ifaceFQN := resolveType(iface, ns, file.Uses)
			if _, exists := ca.bindings[ifaceFQN]; !exists {
				ca.bindings[ifaceFQN] = &ServiceBinding{Abstract: ifaceFQN, Concrete: fqn, Singleton: true, Source: path}
			}
		}
	}
}

func (ca *ContainerAnalyzer) scanDirectory(dir string, handler func(string, string)) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".php") && !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		handler(path, string(content))
		return nil
	})
}

type ComposerAutoload struct {
	PSR4 map[string]string
}

func ParseComposerAutoload(rootPath string) *ComposerAutoload {
	composerPath := filepath.Join(rootPath, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return nil
	}
	var composer struct {
		Autoload struct {
			PSR4 map[string]interface{} `json:"psr-4"`
		} `json:"autoload"`
	}
	if json.Unmarshal(data, &composer) != nil {
		return nil
	}
	result := &ComposerAutoload{PSR4: make(map[string]string)}
	for ns, paths := range composer.Autoload.PSR4 {
		switch v := paths.(type) {
		case string:
			result.PSR4[ns] = v
		case []interface{}:
			if len(v) > 0 {
				if s, ok := v[0].(string); ok {
					result.PSR4[ns] = s
				}
			}
		}
	}
	return result
}

func cleanPHPString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "'\"")
	s = strings.TrimSuffix(s, "::class")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

func resolveType(name, currentNs string, uses []parser.UseNode) string {
	if strings.HasPrefix(name, "\\") {
		return strings.TrimPrefix(name, "\\")
	}
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range uses {
		if u.Alias == parts[0] {
			if len(parts) > 1 {
				return u.FullName + "\\" + parts[1]
			}
			return u.FullName
		}
	}
	if currentNs != "" {
		return currentNs + "\\" + name
	}
	return name
}
