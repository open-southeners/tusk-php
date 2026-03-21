<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;

class Category extends Model
{
    public string $name;
    public string $slug;

    public function products(): array
    {
        return [];
    }
}
