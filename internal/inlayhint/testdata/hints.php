<?php
namespace App;

use Monolog\Logger;
use Monolog\Handler\StreamHandler;

// VarType_SimpleNew: assignment yields a class type hint
$x = new Logger();

// VarType_ScalarSkipped: scalar literals get no hint
$n = 42;
$s = "hello";

// Foreach_ElementType: foreach value variable
$items = ['alpha', 'beta'];
foreach ($items as $item) {
    echo $item;
}

// Foreach_KeyValue: foreach with key => value
$map = ['a' => 1, 'b' => 2];
foreach ($map as $k => $v) {
    echo $k . $v;
}

// ClosureReturn_ArrowFn: arrow function without return type
$fn = fn($x) => new Logger();

// ClosureReturn_AlreadyTyped: arrow function with explicit return type — no hint
$fn2 = fn(): Logger => new Logger();

// ClosureReturn_AnonFn: anonymous function without return type
$fn3 = function() {
    return new Logger();
};

class HintService
{
    /**
     * @return Logger
     */
    public function getLogger()
    {
        return new Logger();
    }

    public function getDeclaredLogger(): Logger
    {
        return new Logger();
    }

    public function twoArgs(Logger $a, Logger $b): void
    {
    }

    public function oneArg(Logger $a): void
    {
    }

    public function namedMatch(Logger $name): void
    {
    }
}

// ParamName_Basic: two-arg method call — hints for both
$svc = new HintService();
$loggerA = new Logger();
$loggerB = new Logger();
$svc->twoArgs($loggerA, $loggerB);

// ParamName_SuppressSingle: one-arg method call — no hint with SuppressSingleParam=true
$svc->oneArg($loggerA);

// ParamName_SuppressNameMatch: arg variable name matches param name — no hint with SuppressNameMatch=true
$name = new Logger();
$svc->namedMatch($name);

// ParamName_NamedArgSkip: named argument — no hint
$svc->oneArg(a: $loggerA);

// ParamName_MethodCall: instance method call with two params
$svc->twoArgs($loggerA, $loggerB);
